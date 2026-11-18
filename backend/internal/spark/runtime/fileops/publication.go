package fileops

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"
	"syscall"

	"campfire/internal/shared/model"

	"golang.org/x/sys/unix"
)

// DestinationSnapshot never follows links, including intermediate components.
// Missing or blocked parents are represented so a directory conflict can be
// skipped/renamed without reading through that parent.
// DestinationSnapshot inspects a destination without following links.
func DestinationSnapshot(root *os.Root, name string) (*model.FileEntry, string, error) {
	parts := strings.Split(name, "/")
	for index := range parts {
		current := strings.Join(parts[:index+1], "/")

		info, err := root.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) {
			return nil, "absent", nil
		}

		if err != nil {
			return nil, "", Failure(model.FileIO, fmt.Sprintf("Could not inspect destination %q.", current))
		}

		if index < len(parts)-1 && !info.IsDir() {
			return nil, "blocked", nil
		}

		if index == len(parts)-1 {
			kind := model.FileKindUnsupported

			revision := StatRevision(info)
			if info.IsDir() {
				kind = model.FileKindDirectory

				if stat, ok := info.Sys().(*syscall.Stat_t); ok {
					revision = fmt.Sprintf("directory:%d:%d", stat.Dev, stat.Ino)
				}
			} else if info.Mode().IsRegular() {
				kind = model.FileKindFile
			}

			return &model.FileEntry{Name: path.Base(name), Kind: kind, Mode: uint32(info.Mode().Perm()), Size: info.Size(), ModifiedAt: info.ModTime().UTC()}, revision, nil
		}
	}

	return nil, "", Failure(model.FileInvalid, "Invalid destination.")
}

// PublishStagedFile publishes a revision-checked upload without following links.
// Extraction intentionally writes directly instead of using staged publication.
func PublishStagedFile(ctx context.Context, root *os.Root, source, destination, revision string) error {
	if err := ContainedPath(root, source, false); err != nil {
		return err
	}

	staged, err := root.Lstat(source)
	if err != nil || !staged.Mode().IsRegular() {
		return Failure(model.FileConflict, "Staged file changed or is not a regular file.")
	}

	if !ValidPath(destination, false) {
		return Failure(model.FileInvalid, "Invalid publication destination.")
	}

	if err := ContainedPath(root, destination, true); err != nil {
		return err
	}

	existing, current, err := DestinationSnapshot(root, destination)
	if err != nil {
		return err
	}

	if current != revision || existing != nil && existing.Kind != model.FileKindFile {
		return Failure(model.FileConflict, fmt.Sprintf("Destination %q changed or already exists.", destination))
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if revision == "absent" {
		if err := RenameNoReplace(root, source, destination); err != nil {
			if errors.Is(err, fs.ErrExist) {
				return Failure(model.FileConflict, fmt.Sprintf("Destination %q already exists.", destination))
			}

			return Failure(model.FileIO, fmt.Sprintf("Could not publish %q.", destination))
		}

		return nil
	}

	if err := root.Rename(source, destination); err != nil {
		return Failure(model.FileIO, fmt.Sprintf("Could not publish %q.", destination))
	}

	return nil
}

// RenameNoReplace pins directory descriptors and uses RENAME_NOREPLACE to
// publish an absent destination without replacing a new arrival.
func RenameNoReplace(root *os.Root, source, destination string) error {
	from, err := root.OpenFile(path.Dir(source), os.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer func() { _ = from.Close() }()

	to, err := root.OpenFile(path.Dir(destination), os.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}

	defer func() { _ = to.Close() }()

	return unix.Renameat2(int(from.Fd()), path.Base(source), int(to.Fd()), path.Base(destination), unix.RENAME_NOREPLACE)
}
