package runtime

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"campfire/internal/shared/model"
)

const backupMetadataLimit = 128 << 10

type backupManifest struct {
	Format int `json:"format"`
	model.BackupOptions
	Missing []string `json:"missing,omitempty"`
}

func (m backupManifest) Validate() error {
	if m.Format != 1 || m.Mode == "" {
		return errors.New("unsupported backup metadata format")
	}

	if err := m.BackupOptions.Validate(); err != nil {
		return err
	}

	seen := make(map[string]bool)

	for _, name := range m.Missing {
		optional := false
		for _, item := range m.Paths {
			optional = optional || item.Path == name && !item.Required
		}

		if !optional || seen[name] {
			return errors.New("invalid missing save-data path")
		}

		seen[name] = true
	}

	return nil
}

func readBackupManifest(input io.Reader) (backupManifest, error) {
	manifest := backupManifest{Format: 1, BackupOptions: model.BackupOptions{Mode: model.BackupModeFull}}

	compressed, err := gzip.NewReader(input)
	if err != nil {
		return manifest, err
	}

	defer func() { _ = compressed.Close() }()

	archive := tar.NewReader(compressed)

	header, err := archive.Next()
	if errors.Is(err, io.EOF) {
		return manifest, nil
	}

	if err != nil {
		return manifest, err
	}

	if header.Name != model.BackupMetadataName {
		return manifest, nil
	}

	if header.Typeflag != tar.TypeReg || header.Size < 1 || header.Size > backupMetadataLimit {
		return manifest, errors.New("invalid backup metadata")
	}

	data, err := io.ReadAll(archive)
	if err != nil {
		return manifest, err
	}

	manifest = backupManifest{}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return manifest, err
	}

	if manifest.Format != 1 || manifest.Mode == "" {
		return manifest, errors.New("unsupported backup metadata format")
	}

	return manifest, manifest.Validate()
}

func writeBackupManifest(archive *tar.Writer, root *os.Root, options model.BackupOptions) error {
	options.Mode = options.Mode.Normalized()
	if err := options.Validate(); err != nil {
		return err
	}

	manifest := backupManifest{Format: 1, BackupOptions: options}
	for _, item := range options.Paths {
		err := savedataExists(root, item.Path)
		if errors.Is(err, os.ErrNotExist) && !item.Required {
			manifest.Missing = append(manifest.Missing, item.Path)
			continue
		}

		if err != nil {
			return fmt.Errorf("save-data path %q is unavailable: %w", item.Path, err)
		}
	}

	if options.Mode == model.BackupModeSavedata && len(manifest.Missing) == len(options.Paths) {
		return errors.New("no declared save-data paths exist")
	}

	data, err := json.Marshal(manifest)
	if err != nil {
		return err
	}

	if err := archive.WriteHeader(&tar.Header{Name: model.BackupMetadataName, Typeflag: tar.TypeReg, Mode: 0600, Size: int64(len(data))}); err != nil {
		return err
	}

	_, err = archive.Write(data)

	return err
}

func savedataExists(root *os.Root, name string) error {
	parts := strings.Split(name, "/")
	for i := range parts {
		info, err := root.Lstat(strings.Join(parts[:i+1], "/"))
		if err != nil {
			return err
		}

		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() && !info.Mode().IsRegular() {
			return errors.New("save-data paths cannot contain links or special files")
		}
	}

	return nil
}

// preserveInstallation copies unselected data into the staged restore. Links are
// copied as links, never followed; the original tree remains untouched on failure.
func preserveInstallation(ctx context.Context, volume, stage string, options model.BackupOptions, limit int64) error {
	source, err := os.OpenRoot(volume)
	if err != nil {
		return err
	}
	defer func() { _ = source.Close() }()

	target, err := os.OpenRoot(stage)
	if err != nil {
		return err
	}
	defer func() { _ = target.Close() }()

	var total int64

	count := 0

	if err := fs.WalkDir(target.FS(), ".", func(_ string, item fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		info, err := item.Info()
		if err == nil && info.Mode().IsRegular() {
			total += info.Size()
		}

		return err
	}); err != nil {
		return err
	}

	return fs.WalkDir(source.FS(), ".", func(name string, item fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		if name == "." {
			return nil
		}

		if options.Includes(name) {
			if item.IsDir() {
				return fs.SkipDir
			}

			return nil
		}

		count++
		if count > backupEntries {
			return errors.New("installation exceeds restore entry limit")
		}

		if err := target.MkdirAll(filepath.Dir(name), 0700); err != nil {
			return err
		}

		if item.Type()&os.ModeSymlink != 0 {
			link, err := source.Readlink(name)
			if err != nil {
				return err
			}

			return target.Symlink(link, name)
		}

		if item.IsDir() {
			return target.MkdirAll(name, 0700)
		}

		input, err := source.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if err != nil {
			return err
		}
		defer func() { _ = input.Close() }()

		info, err := input.Stat()
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("installation contains an unsupported file")
		}

		total += info.Size()
		if total > limit {
			return errors.New("restored data exceeds allocation")
		}

		output, err := target.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
		if err != nil {
			return err
		}
		defer func() { _ = output.Close() }()

		if _, err := io.CopyN(output, backupReader{ctx, input}, info.Size()); err != nil {
			return err
		}

		return output.Sync()
	})
}
