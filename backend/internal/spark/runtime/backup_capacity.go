package runtime

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"strings"

	"campfire/internal/shared/model"
)

const backupArchiveLimit = 1000
const backupUploadSessions = 2

// Bound directory reads while allowing pending uploads beside a full catalog.
// Staging files reserve capacity but are not completed backups.
func backupDirectoryEntries(root *os.Root) ([]fs.DirEntry, error) {
	directory, err := root.Open(".")
	if err != nil {
		return nil, fileFailure(model.FileIO, "Could not list backups.")
	}
	defer func() { _ = directory.Close() }()

	entries, err := directory.ReadDir(backupArchiveLimit + backupUploadSessions + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fileFailure(model.FileIO, "Could not list backups.")
	}

	completed := 0

	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), backupUploadPrefix) {
			completed++
		}
	}

	if completed > backupArchiveLimit || len(entries) > backupArchiveLimit+backupUploadSessions {
		return nil, fileFailure(model.FileLimitExceeded, "Backup storage exceeds the 1000-backup limit.")
	}

	return entries, nil
}

func checkBackupCapacity(directory string, additional int) error {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return fileFailure(model.FileIO, "Backup storage unavailable.")
	}
	defer func() { _ = root.Close() }()

	entries, err := backupDirectoryEntries(root)
	if err != nil {
		return err
	}

	if len(entries)+additional > backupArchiveLimit {
		return fileFailure(model.FileLimitExceeded, "Backup storage has reached the 1000-backup limit, including pending uploads.")
	}

	return nil
}
