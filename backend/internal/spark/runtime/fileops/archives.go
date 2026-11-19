package fileops

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
	"syscall"

	"campfire/internal/shared/model"
	"campfire/internal/shared/workprogress"
	"campfire/internal/spark/runtime/fileops/archive"
)

func (m *Manager) archiveFiles(ctx context.Context, d model.Deployment, root *os.Root, volume string, q model.FileRequest) (model.FileResult, error) {
	result := model.FileResult{ServerID: q.ServerID, Path: q.Destination}
	if err := ctx.Err(); err != nil {
		return result, err
	}

	sources := []string{q.Path}
	if len(q.Paths) > 0 {
		if q.Operation != model.FileOperationCompress || len(q.Paths) > 1000 {
			return result, Failure(model.FileInvalid, "Select at most 1,000 archive items.")
		}

		sources = q.Paths

		seen := make(map[string]bool, len(sources))
		for _, source := range sources {
			if !ValidPath(source, false) || path.Dir(source) != q.Path || seen[source] {
				return result, Failure(model.FileInvalid, "Select distinct items from the current directory.")
			}

			seen[source] = true
		}
	}

	for _, source := range sources {
		if source == q.Destination || strings.HasPrefix(q.Destination, source+"/") {
			return result, Failure(model.FileInvalid, "Choose a destination outside the selected items.")
		}

		if err := ContainedPath(root, source, false); err != nil {
			return result, err
		}
	}

	if !ValidPath(q.Destination, false) {
		return result, Failure(model.FileInvalid, "Choose a new destination outside the source directory.")
	}

	if err := ContainedPath(root, q.Path, false); err != nil {
		return result, err
	}

	if err := ContainedPath(root, q.Destination, true); err != nil {
		return result, err
	}

	if _, err := root.Lstat(q.Destination); !errors.Is(err, os.ErrNotExist) {
		return result, Failure(model.FileConflict, fmt.Sprintf("Destination %q already exists. Choose a new name; existing entries are not merged or overwritten.", q.Destination))
	}

	if q.Operation == model.FileOperationExtract {
		if err := root.Mkdir(q.Destination, 0700); err != nil {
			return result, Failure(model.FileConflict, "Destination appeared before extraction started.")
		}

		if err := m.ownExtractionDirectory(root, d, q.Destination); err != nil {
			return result, err
		}

		if err := m.extractFiles(ctx, d, root, q.Path, q.Destination); err != nil {
			return result, ArchiveFailure(err)
		}

		return result, nil
	}

	temporary := ArchiveStagingPrefix + rand.Text()
	defer func() { _ = root.RemoveAll(temporary) }()

	if err := m.compressFiles(ctx, d, root, sources, temporary, q.Destination); err != nil {
		return result, ArchiveFailure(err)
	}

	workprogress.New(ctx, workprogress.Committing, 0).Flush()

	if err := CheckWriteCapacity(ctx, volume, d.Configuration.DiskMB, 0, 0); err != nil {
		return result, err
	}

	if err := ctx.Err(); err != nil {
		return result, err
	}

	if err := RenameNoReplace(root, temporary, q.Destination); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return result, Failure(model.FileConflict, "Destination appeared during archive preparation. Nothing was replaced.")
		}

		return result, Failure(model.FileIO, "Could not publish archive result.")
	}

	return result, nil
}

// ArchiveFailure preserves actionable file errors and normalizes decoder failures.
func ArchiveFailure(err error) error {
	var failure *model.FileError
	if errors.As(err, &failure) {
		return err
	}

	return Failure(model.FileIO, "Archive operation failed. Check the archive, disk allocation and Spark logs.")
}

func (m *Manager) compressFiles(ctx context.Context, d model.Deployment, root *os.Root, sources []string, temporary, destination string) error {
	file, err := root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	output := &limitedArchiveWriter{Writer: file}

	encoder, err := archive.NewWriter(ctx, output, destination)
	if err != nil {
		return err
	}

	var total int64

	count := 0

	for _, source := range sources {
		err = fs.WalkDir(root.FS(), source, func(name string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}

			if err := ctx.Err(); err != nil {
				return err
			}

			if ReservedPath(name) {
				if entry.IsDir() {
					return fs.SkipDir
				}

				return nil
			}

			if name == "." {
				return nil
			}

			count++
			if count > archiveEntryLimit || !ValidPath(name, false) {
				return Failure(model.FileLimitExceeded, "Archives are limited to 100,000 entries.")
			}

			if err := ContainedPath(root, name, false); err != nil {
				return err
			}

			input, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
			if err != nil {
				return err
			}

			defer func() { _ = input.Close() }()

			info, err := input.Stat()
			if err != nil {
				return err
			}

			if !info.IsDir() && !info.Mode().IsRegular() {
				return Failure(model.FileInvalid, "Archives cannot contain links or special files.")
			}

			total += info.Size()
			if total > model.FileTransferLimit {
				return Failure(model.FileLimitExceeded, "Archive contents exceed 100 GiB.")
			}

			member := archive.Entry{Name: name, Size: info.Size(), Mode: info.Mode(), Modified: info.ModTime()}
			if source != "." {
				member.Name = strings.TrimPrefix(name, path.Dir(source)+"/")
			}

			if info.IsDir() {
				member.Size = 0
			}

			if err := encoder.WriteHeader(member); err != nil {
				return err
			}

			if info.Mode().IsRegular() {
				_, err = io.CopyN(encoder, contextReader{ctx, input}, info.Size())
			}

			return err
		})
		if err != nil {
			break
		}
	}

	closeArchive := encoder.Close()

	if err != nil {
		return err
	}

	if closeArchive != nil {
		return closeArchive
	}

	if err := m.FileOwner(file, d.Ember.User); err != nil {
		return err
	}

	return file.Sync()
}

func (m *Manager) extractFiles(ctx context.Context, d model.Deployment, root *os.Root, source, destination string) error {
	input, err := root.OpenFile(source, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer func() { _ = input.Close() }()

	info, err := input.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return Failure(model.FileInvalid, "Select a regular archive file.")
	}

	if info.Size() > model.FileTransferLimit {
		return Failure(model.FileLimitExceeded, "Archive exceeds 100 GiB.")
	}

	allocation, err := newArchiveAllocation(ctx, root.Name(), d.Configuration.DiskMB)
	if err != nil {
		return err
	}

	target, err := root.OpenRoot(destination)
	if err != nil {
		return err
	}

	defer func() { _ = target.Close() }()

	members := newArchiveMembers(archiveEntryLimit, model.FileTransferLimit)
	mkdir := func(name string) error {
		if err := target.MkdirAll(name, 0700); err != nil {
			return err
		}

		for current := name; current != "."; current = path.Dir(current) {
			if err := m.ownExtractionDirectory(target, d, current); err != nil {
				return err
			}
		}

		return nil
	}
	unpack := func(entry archive.Entry, reader io.Reader) error {
		name, size, mode := entry.Name, entry.Size, entry.Mode

		if err := ctx.Err(); err != nil {
			return err
		}

		name, err := members.accept(name, size, mode)
		if err != nil || name == "" {
			return err
		}

		if mode.IsDir() {
			return mkdir(name)
		}

		if err := allocation.reserve(ctx, root.Name(), 0, size); err != nil {
			return err
		}

		if err := mkdir(path.Dir(name)); err != nil {
			return err
		}

		file, err := target.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}

		defer func() { _ = file.Close() }()

		if err := m.FileOwner(file, d.Ember.User); err != nil {
			return err
		}

		if _, err := io.CopyN(file, contextReader{ctx, reader}, size); err != nil {
			return err
		}

		if err := file.Chmod(ImportedMode(mode)); err != nil {
			return err
		}

		return file.Sync()
	}

	return ReadArchive(ctx, input, source, unpack)
}
