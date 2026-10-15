package runtime

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"unicode"

	"campfire/internal/model"
)

func fileFailure(code, message string) error { return &model.FileError{Code: code, Message: message} }

func filePath(value string, rootAllowed bool) bool {
	if value == "." {
		return rootAllowed
	}

	return len(value) > 0 && len(value) <= 1024 && fs.ValidPath(value) && !strings.ContainsAny(value, "\\\x00") && !strings.ContainsFunc(value, unicode.IsControl)
}

func containedPath(root *os.Root, value string, allowMissing bool) error {
	if value == "." {
		return nil
	}

	parts := strings.Split(value, "/")
	for index := range parts {
		info, err := root.Lstat(strings.Join(parts[:index+1], "/"))
		if errors.Is(err, os.ErrNotExist) && allowMissing && index == len(parts)-1 {
			return nil
		}

		if err != nil {
			return fileFailure("file_not_found", "File or directory not found.")
		}

		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return fileFailure("invalid_file", "Symbolic links and special files are not accessible.")
		}
	}

	return nil
}

func readFile(root *os.Root, name string) ([]byte, os.FileInfo, error) {
	file, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, nil, fileFailure("file_not_found", "File not found.")
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, nil, fileFailure("invalid_file", "Select a regular file.")
	}

	if info.Size() > model.FileLimit {
		return nil, nil, fileFailure("file_limit", "Transfers are limited to 1 MiB per file.")
	}

	data, err := io.ReadAll(io.LimitReader(file, model.FileLimit+1))
	if err != nil {
		return nil, nil, fileFailure("file_io", "Could not read file.")
	}

	if len(data) > model.FileLimit {
		return nil, nil, fileFailure("file_limit", "Transfers are limited to 1 MiB per file.")
	}

	return data, info, nil
}

func fileRevision(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

// Files executes contained file operations serialized against server lifecycle work.
func (m *Manager) Files(ctx context.Context, d model.Deployment, q model.FileRequest) (model.FileResult, error) {
	if strings.HasPrefix(q.Operation, "backup.") {
		return m.backupFiles(ctx, d, q)
	}

	if err := ctx.Err(); err != nil {
		return model.FileResult{}, err
	}

	if !safeID.MatchString(q.ServerID) || q.ServerID != d.ServerID || !filePath(q.Path, q.Operation == "list") {
		return model.FileResult{}, fileFailure("invalid_file", "Invalid file request.")
	}

	if len(q.Data) > model.FileLimit {
		return model.FileResult{}, fileFailure("file_limit", "Transfers are limited to 1 MiB per file.")
	}

	switch q.Operation {
	case "list", "read", "write", "mkdir", "rename", "delete":
	default:
		return model.FileResult{}, fileFailure("invalid_file", "Unsupported file operation.")
	}

	e, err := m.load(q.ServerID)
	if err != nil {
		return model.FileResult{}, fileFailure("file_io", "Runtime unavailable.")
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.Status.Busy {
		return model.FileResult{}, fileFailure("file_busy", "Wait for the current server operation to finish.")
	}

	mutate := q.Operation != "list" && q.Operation != "read"
	if mutate {
		state, err := m.inspect(ctx, q.ServerID)
		if err != nil {
			return model.FileResult{}, fileFailure("file_io", "Could not verify server state.")
		}

		if state != "missing" && state != "exited" && state != "created" {
			return model.FileResult{}, fileFailure("file_busy", "Stop the server before modifying files.")
		}
	}

	volume, err := m.volume(q.ServerID)
	if err != nil {
		return model.FileResult{}, fileFailure("file_io", "Game data unavailable.")
	}

	root, err := os.OpenRoot(volume)
	if err != nil {
		return model.FileResult{}, fileFailure("file_not_found", "Install the server to prepare its game data directory.")
	}

	defer func() { _ = root.Close() }()

	if err := containedPath(root, q.Path, q.Operation == "write" || q.Operation == "mkdir"); err != nil {
		return model.FileResult{}, err
	}

	result := model.FileResult{ServerID: q.ServerID, Path: q.Path}
	switch q.Operation {
	case "list":
		directory, err := root.OpenFile(q.Path, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
		if err != nil {
			return result, fileFailure("file_not_found", "Directory not found.")
		}
		defer func() { _ = directory.Close() }()

		children, err := directory.ReadDir(1001)
		if err != nil && !errors.Is(err, io.EOF) {
			return result, fileFailure("invalid_file", "Select a directory.")
		}

		if len(children) > 1000 {
			return result, fileFailure("file_limit", "Directories are limited to 1000 entries per listing.")
		}

		result.Entries = make([]model.FileEntry, 0, len(children))
		for _, child := range children {
			info, err := root.Lstat(path.Join(q.Path, child.Name()))
			if err != nil {
				return result, fileFailure("file_io", "Directory changed; refresh and try again.")
			}

			kind := "file"
			if info.IsDir() {
				kind = "directory"
			} else if !info.Mode().IsRegular() {
				kind = "unsupported"
			}

			result.Entries = append(result.Entries, model.FileEntry{Name: child.Name(), Kind: kind, Size: info.Size(), ModifiedAt: info.ModTime().UTC()})
		}

		sort.Slice(result.Entries, func(i, j int) bool {
			a, b := result.Entries[i], result.Entries[j]
			if (a.Kind == "directory") != (b.Kind == "directory") {
				return a.Kind == "directory"
			}

			return a.Name < b.Name
		})
	case "read":
		data, _, err := readFile(root, q.Path)
		if err != nil {
			return result, err
		}

		result.Data = data
		result.Revision = fileRevision(data)
	case "write":
		revision := "absent"
		mode := fs.FileMode(0600)
		oldSize := int64(0)

		if _, err := root.Lstat(q.Path); err == nil {
			data, info, err := readFile(root, q.Path)
			if err != nil {
				return result, err
			}

			revision = fileRevision(data)
			mode = info.Mode().Perm()
			oldSize = info.Size()
		} else if !errors.Is(err, os.ErrNotExist) {
			return result, fileFailure("file_io", "Could not inspect file.")
		}

		if q.Revision != revision {
			return result, fileFailure("file_conflict", "File changed or already exists. Reload before saving.")
		}

		if err := checkWriteCapacity(ctx, volume, d.Configuration.DiskMB, oldSize, int64(len(q.Data))); err != nil {
			return result, err
		}

		temporary := path.Join(path.Dir(q.Path), ".campfire-"+rand.Text())

		file, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if err != nil {
			return result, fileFailure("file_io", "Could not prepare file.")
		}
		defer func() { _ = root.Remove(temporary) }()

		_, writeErr := file.Write(q.Data)
		ownerErr := m.FileOwner(file, d.Ember.User)
		syncErr := file.Sync()

		closeErr := file.Close()
		if writeErr != nil || ownerErr != nil || syncErr != nil || closeErr != nil {
			return result, fileFailure("file_io", "Could not save file or assign game ownership.")
		}

		if err := ctx.Err(); err != nil {
			return result, err
		}

		if err := root.Rename(temporary, q.Path); err != nil {
			return result, fileFailure("file_io", "Could not replace file.")
		}

		result.Revision = fileRevision(q.Data)
	case "mkdir":
		if err := root.Mkdir(q.Path, 0700); err != nil {
			return result, fileFailure("file_conflict", "Directory already exists or parent is unavailable.")
		}

		directory, err := root.Open(q.Path)
		if err != nil {
			return result, fileFailure("file_io", "Could not open new directory.")
		}

		ownerErr := m.FileOwner(directory, d.Ember.User)
		_ = directory.Close()

		if ownerErr != nil {
			_ = root.Remove(q.Path)
			return result, fileFailure("file_io", "Could not assign directory ownership.")
		}
	case "rename":
		if !filePath(q.Destination, false) {
			return result, fileFailure("invalid_file", "Invalid destination.")
		}

		if err := containedPath(root, q.Destination, true); err != nil {
			return result, err
		}

		if _, err := root.Lstat(q.Destination); !errors.Is(err, os.ErrNotExist) {
			return result, fileFailure("file_conflict", "Destination already exists.")
		}

		if err := root.Rename(q.Path, q.Destination); err != nil {
			return result, fileFailure("file_io", "Could not rename file or directory.")
		}

		result.Path = q.Destination
	case "delete":
		if err := root.Remove(q.Path); err != nil {
			return result, fileFailure("file_conflict", "Could not delete entry. Directories must be empty.")
		}
	}

	return result, nil
}

func assignFileOwner(file *os.File, user string) error {
	parts := strings.Split(user, ":")

	uid, err := strconv.Atoi(parts[0])
	if err != nil || uid < 1 {
		return errors.New("invalid game user")
	}

	gid := uid
	if len(parts) == 2 {
		gid, err = strconv.Atoi(parts[1])
		if err != nil {
			return err
		}
	}

	return file.Chown(uid, gid)
}

func checkWriteCapacity(ctx context.Context, directory string, limitMB, oldSize, newSize int64) error {
	var used int64

	root, err := os.OpenRoot(directory)
	if err != nil {
		return fileFailure("file_io", "Could not verify disk allocation.")
	}

	defer func() { _ = root.Close() }()

	err = fs.WalkDir(root.FS(), ".", func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		if entry.Type().IsRegular() {
			info, err := entry.Info()
			if err != nil {
				return err
			}

			used += info.Size()
		}

		return nil
	})
	if err != nil {
		return fileFailure("file_io", "Could not verify disk allocation.")
	}

	if limitMB < 1 || used-oldSize+newSize > limitMB*1024*1024 {
		return fileFailure("file_limit", "File would exceed the server disk allocation.")
	}

	return nil
}
