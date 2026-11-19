package fileops

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

	"campfire/internal/shared/model"
	"campfire/internal/shared/utils"
)

// Failure builds a transport-independent file-operation error.
func Failure(code model.FileErrorCode, message string) error {
	return &model.FileError{Code: code, Message: message}
}

// ValidPath validates a relative game-data path.
func ValidPath(value string, rootAllowed bool) bool {
	if ReservedPath(value) {
		return false
	}

	if value == "." {
		return rootAllowed
	}

	return len(value) > 0 && len(value) <= 1024 && fs.ValidPath(value) && !strings.ContainsAny(value, "\\\x00") && !strings.ContainsFunc(value, unicode.IsControl)
}

// ContainedPath rejects links and unsafe intermediate path components.
func ContainedPath(root *os.Root, value string, allowMissing bool) error {
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
			return Failure(model.FileNotFound, "File or directory not found.")
		}

		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return Failure(model.FileInvalid, "Symbolic links and special files are not accessible.")
		}
	}

	return nil
}

// ReadFile reads a bounded regular editor file without following links.
func ReadFile(root *os.Root, name string) ([]byte, os.FileInfo, error) {
	file, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, nil, Failure(model.FileNotFound, "File not found.")
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, nil, Failure(model.FileInvalid, "Select a regular file.")
	}

	if info.Size() > model.FileLimit {
		return nil, nil, Failure(model.FileLimitExceeded, "Text reads are limited to 1 MiB; use a chunked download for larger files.")
	}

	data, err := io.ReadAll(io.LimitReader(file, model.FileLimit+1))
	if err != nil {
		return nil, nil, Failure(model.FileIO, "Could not read file.")
	}

	if len(data) > model.FileLimit {
		return nil, nil, Failure(model.FileLimitExceeded, "Text reads are limited to 1 MiB; use a chunked download for larger files.")
	}

	return data, info, nil
}

// FileRevision identifies the current editor contents.
func FileRevision(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

// Files executes contained file operations serialized against server lifecycle work.
// Files dispatches bounded server-file requests within the game-data root.
func (m *Manager) Files(ctx context.Context, d model.Deployment, volume string, e *State, q model.FileRequest) (model.FileResult, error) {
	if err := ctx.Err(); err != nil {
		return model.FileResult{}, err
	}

	if !utils.IsValidResourceID(q.ServerID) || q.ServerID != d.ServerID || !ValidPath(q.Path, q.Operation == model.FileOperationList || q.Operation == model.FileOperationStat || q.Operation == model.FileOperationCompress || q.Operation.IsCompression()) {
		return model.FileResult{}, Failure(model.FileInvalid, "Invalid file request.")
	}

	if len(q.Data) > model.FileLimit {
		return model.FileResult{}, Failure(model.FileLimitExceeded, "File data is limited to 1 MiB per command; use chunked uploads for larger files.")
	}

	if q.Operation.IsCompression() {
		return m.compression(ctx, d, volume, q)
	}

	if m.compressionActive(q.ServerID) && !q.Operation.IsFileControl() {
		return model.FileResult{}, Failure(model.FileBusy, "Wait for compression to finish.")
	}

	switch q.Operation {
	case model.FileOperationList, model.FileOperationRead, model.FileOperationWrite, model.FileOperationMkdir, model.FileOperationRename, model.FileOperationDelete, model.FileOperationChmod, model.FileOperationStat, model.FileOperationReadChunk, model.FileOperationUploadBegin, model.FileOperationUploadChunk, model.FileOperationUploadCommit, model.FileOperationUploadCancel, model.FileOperationCompress, model.FileOperationExtract, model.FileOperationExtractStart, model.FileOperationExtractStatus, model.FileOperationExtractResolve, model.FileOperationExtractCancel:
	default:
		return model.FileResult{}, Failure(model.FileInvalid, "Unsupported file operation.")
	}

	if q.Operation.IsExtraction() {
		return m.Run(ctx, d, volume, q)
	}

	root, err := os.OpenRoot(volume)
	if err != nil {
		return model.FileResult{}, Failure(model.FileNotFound, "Install the server to prepare its game data directory.")
	}

	defer func() { _ = root.Close() }()

	m.pruneUploads(root, e)

	if q.Operation.IsUpload() {
		return m.uploadFile(ctx, d, root, volume, e, q)
	}

	if q.Operation == model.FileOperationCompress || q.Operation == model.FileOperationExtract {
		return m.archiveFiles(ctx, d, root, volume, q)
	}

	if err := ContainedPath(root, q.Path, q.Operation == model.FileOperationWrite || q.Operation == model.FileOperationMkdir); err != nil {
		return model.FileResult{}, err
	}

	result := model.FileResult{ServerID: q.ServerID, Path: q.Path}
	switch q.Operation {
	case model.FileOperationStat, model.FileOperationReadChunk:
		return chunkFile(root, q)
	case model.FileOperationList:
		directory, err := root.OpenFile(q.Path, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
		if err != nil {
			return result, Failure(model.FileNotFound, "Directory not found.")
		}
		defer func() { _ = directory.Close() }()

		children, err := directory.ReadDir(1001)
		if err != nil && !errors.Is(err, io.EOF) {
			return result, Failure(model.FileInvalid, "Select a directory.")
		}

		if len(children) > 1000 {
			return result, Failure(model.FileLimitExceeded, "Directories are limited to 1000 entries per listing.")
		}

		result.Entries = make([]model.FileEntry, 0, len(children))
		for _, child := range children {
			if ReservedPath(child.Name()) {
				continue
			}

			info, err := root.Lstat(path.Join(q.Path, child.Name()))
			if err != nil {
				return result, Failure(model.FileIO, "Directory changed; refresh and try again.")
			}

			kind := model.FileKindFile
			if info.IsDir() {
				kind = model.FileKindDirectory
			} else if !info.Mode().IsRegular() {
				kind = model.FileKindUnsupported
			}

			result.Entries = append(result.Entries, model.FileEntry{Name: child.Name(), Kind: kind, Mode: uint32(info.Mode().Perm()), Size: info.Size(), ModifiedAt: info.ModTime().UTC()})
		}

		sort.Slice(result.Entries, func(i, j int) bool {
			a, b := result.Entries[i], result.Entries[j]
			if (a.Kind == model.FileKindDirectory) != (b.Kind == model.FileKindDirectory) {
				return a.Kind == model.FileKindDirectory
			}

			return a.Name < b.Name
		})
	case model.FileOperationRead:
		data, _, err := ReadFile(root, q.Path)
		if err != nil {
			return result, err
		}

		result.Data = data
		result.Revision = FileRevision(data)
	case model.FileOperationWrite:
		revision := "absent"
		mode := fs.FileMode(0600)
		oldSize := int64(0)

		if _, err := root.Lstat(q.Path); err == nil {
			data, info, err := ReadFile(root, q.Path)
			if err != nil {
				return result, err
			}

			revision = FileRevision(data)
			mode = info.Mode().Perm()
			oldSize = info.Size()
		} else if !errors.Is(err, os.ErrNotExist) {
			return result, Failure(model.FileIO, "Could not inspect file.")
		}

		if q.Revision != revision {
			return result, Failure(model.FileConflict, "File changed or already exists. Reload before saving.")
		}

		if err := CheckWriteCapacity(ctx, volume, d.Configuration.DiskMB, oldSize, int64(len(q.Data))); err != nil {
			return result, err
		}

		temporary := path.Join(path.Dir(q.Path), ".campfire-"+rand.Text())

		file, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if err != nil {
			return result, Failure(model.FileIO, "Could not prepare file.")
		}
		defer func() { _ = root.Remove(temporary) }()

		_, writeErr := file.Write(q.Data)
		ownerErr := m.FileOwner(file, d.Ember.User)
		syncErr := file.Sync()

		closeErr := file.Close()
		if writeErr != nil || ownerErr != nil || syncErr != nil || closeErr != nil {
			return result, Failure(model.FileIO, "Could not save file or assign game ownership.")
		}

		if err := ctx.Err(); err != nil {
			return result, err
		}

		if err := root.Rename(temporary, q.Path); err != nil {
			return result, Failure(model.FileIO, "Could not replace file.")
		}

		result.Revision = FileRevision(q.Data)
	case model.FileOperationMkdir:
		if err := root.Mkdir(q.Path, 0700); err != nil {
			return result, Failure(model.FileConflict, "Directory already exists or parent is unavailable.")
		}

		directory, err := root.Open(q.Path)
		if err != nil {
			return result, Failure(model.FileIO, "Could not open new directory.")
		}

		ownerErr := m.FileOwner(directory, d.Ember.User)
		_ = directory.Close()

		if ownerErr != nil {
			_ = root.Remove(q.Path)
			return result, Failure(model.FileIO, "Could not assign directory ownership.")
		}
	case model.FileOperationRename:
		if !ValidPath(q.Destination, false) {
			return result, Failure(model.FileInvalid, "Invalid destination.")
		}

		if err := ContainedPath(root, q.Destination, true); err != nil {
			return result, err
		}

		if _, err := root.Lstat(q.Destination); !errors.Is(err, os.ErrNotExist) {
			return result, Failure(model.FileConflict, "Destination already exists.")
		}

		if q.Destination == q.Path || strings.HasPrefix(q.Destination, q.Path+"/") {
			return result, Failure(model.FileInvalid, "Choose a destination outside the source folder.")
		}

		if err := RenameNoReplace(root, q.Path, q.Destination); err != nil {
			if errors.Is(err, os.ErrExist) {
				return result, Failure(model.FileConflict, "Destination already exists.")
			}

			return result, Failure(model.FileIO, "Could not rename file or directory.")
		}

		result.Path = q.Destination
	case model.FileOperationDelete:
		if err := root.RemoveAll(q.Path); err != nil {
			return result, Failure(model.FileConflict, "Could not delete entry or all of its contents. Refresh and try again.")
		}
	case model.FileOperationChmod:
		if q.Mode > 0777 {
			return result, Failure(model.FileInvalid, "Permissions must be between 000 and 777 (octal).")
		}

		file, err := root.OpenFile(q.Path, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
		if err != nil {
			return result, Failure(model.FileIO, "Could not open entry to change permissions.")
		}
		defer func() { _ = file.Close() }()

		info, err := file.Stat()
		if err != nil || !info.IsDir() && !info.Mode().IsRegular() {
			return result, Failure(model.FileInvalid, "Select a regular file or directory.")
		}

		if err := file.Chmod(fs.FileMode(q.Mode)); err != nil {
			return result, Failure(model.FileIO, "Could not change permissions.")
		}
	}

	return result, nil
}

// AssignFileOwner assigns ownership to the non-root game user.
func AssignFileOwner(file *os.File, user string) error {
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

// CheckWriteCapacity checks the configured disk allocation before writing.
func CheckWriteCapacity(ctx context.Context, directory string, limitMB, oldSize, newSize int64) error {
	if limitMB == 0 {
		return nil
	}

	used, err := FileUsage(ctx, directory)
	if err != nil {
		return err
	}

	if limitMB < 1 || used-oldSize+newSize > limitMB*1024*1024 {
		return Failure(model.FileLimitExceeded, "File would exceed the server disk allocation.")
	}

	return nil
}

// FileUsage measures regular-file bytes within game data.
func FileUsage(ctx context.Context, directory string) (int64, error) {
	var used int64

	root, err := os.OpenRoot(directory)
	if err != nil {
		return 0, Failure(model.FileIO, "Could not verify disk allocation.")
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
		return 0, Failure(model.FileIO, "Could not verify disk allocation.")
	}

	return used, nil
}
