package runtime

import (
	"campfire/internal/shared/model"
	"crypto/rand"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

func writeAtomic(path string, data []byte, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	file, err := os.CreateTemp(filepath.Dir(path), ".campfire-*")
	if err != nil {
		return err
	}

	defer func() { _ = os.Remove(file.Name()) }()

	if err := file.Chmod(mode); err != nil {
		_ = file.Close()
		return err
	}

	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}

	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}

	if err := file.Close(); err != nil {
		return err
	}

	return os.Rename(file.Name(), path)
}

func ownership(directory, user string) error {
	parts := strings.Split(user, ":")

	uid, err := strconv.Atoi(parts[0])
	if err != nil || uid < 1 {
		return errors.New("invalid non-root user")
	}

	gid := uid
	if len(parts) == 2 {
		gid, err = strconv.Atoi(parts[1])
		if err != nil {
			return err
		}
	}

	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}

	defer func() { _ = root.Close() }()

	return filepath.WalkDir(directory, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}

		if !entry.IsDir() && !entry.Type().IsRegular() {
			return errors.New("server contains special files")
		}

		rel, err := filepath.Rel(directory, path)
		if err != nil {
			return err
		}

		file, err := root.OpenFile(rel, os.O_RDONLY|syscall.O_NONBLOCK, 0)
		if err != nil {
			return err
		}

		defer func() { _ = file.Close() }()

		info, err := file.Stat()
		if err != nil {
			return err
		}

		if !info.IsDir() && !info.Mode().IsRegular() {
			return errors.New("server contains special files")
		}

		return file.Chown(uid, gid)
	})
}

func diskCheck(directory string, limitMB int64) error {
	var total int64

	return filepath.WalkDir(directory, func(_ string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.Type()&os.ModeSymlink != 0 || entry.IsDir() {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}

		total += info.Size()
		if total > limitMB*1024*1024 {
			return errors.New("server data exceeds its disk allocation")
		}

		return nil
	})
}

func configure(directory string, d model.Deployment) error {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()

	for _, config := range d.Ember.Configs {
		if !model.SafePath(config.Path) {
			return errors.New("unsafe configuration path")
		}

		if err := configParents(root, config.Path, config.Create); err != nil {
			return err
		}

		file, err := root.OpenFile(config.Path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if err != nil && (!config.Create || !errors.Is(err, os.ErrNotExist)) {
			return err
		}

		old := []byte{}

		if file != nil {
			info, err := file.Stat()
			if err != nil || !info.Mode().IsRegular() {
				_ = file.Close()
				return errors.New("configuration must be a regular file")
			}

			var readErr error

			old, readErr = io.ReadAll(io.LimitReader(file, (1<<20)+1))
			_ = file.Close()

			if readErr != nil {
				return readErr
			}
		} else if config.Parser == "json" || config.Parser == "yaml" {
			old = []byte("{}")
		}

		if len(old) > 1<<20 {
			return errors.New("configuration file too large")
		}

		var updated []byte

		err = nil

		values := make(map[string]string, len(config.Values))

		remaining := 1 << 20
		for key, value := range config.Values {
			expanded, err := renderBounded(value, environment(d), remaining)
			if err != nil {
				return err
			}

			values[key] = expanded
			remaining -= len(expanded)
		}

		switch config.Parser {
		case "properties":
			lines := strings.Split(string(old), "\n")
			for i, line := range lines {
				key, _, ok := strings.Cut(line, "=")
				if ok {
					key = strings.TrimSpace(key)
					if value, found := values[key]; found {
						if strings.ContainsAny(value, "\r\n") {
							return errors.New("invalid properties value")
						}

						lines[i] = key + "=" + value
						delete(values, key)
					}
				}
			}

			keys := []string{}
			for key := range values {
				keys = append(keys, key)
			}

			sort.Strings(keys)

			for _, key := range keys {
				if strings.ContainsAny(values[key], "\r\n") {
					return errors.New("invalid properties value")
				}

				lines = append(lines, key+"="+values[key])
			}

			updated = []byte(strings.Join(lines, "\n"))
		case "json", "yaml":
			updated, err = structuredConfig(old, config, values)
		case "ini":
			updated, err = iniConfig(old, values)
		case "file":
			updated, err = textConfig(old, values)
		default:
			return errors.New("unsupported configuration parser")
		}

		if err != nil {
			return err
		}

		if len(updated) > 1<<20 {
			return errors.New("rewritten configuration exceeds 1 MiB")
		}

		temporary := filepath.Join(filepath.Dir(config.Path), ".campfire-"+rand.Text())

		output, err := root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}

		_, writeErr := output.Write(updated)
		syncErr := output.Sync()

		closeErr := output.Close()
		if writeErr != nil || syncErr != nil || closeErr != nil {
			_ = root.Remove(temporary)
			return errors.Join(writeErr, syncErr, closeErr)
		}

		if err := root.Rename(temporary, config.Path); err != nil {
			_ = root.Remove(temporary)
			return err
		}
	}

	return nil
}
