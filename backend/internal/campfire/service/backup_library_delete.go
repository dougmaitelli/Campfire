package service

import (
	"context"
	"errors"
	"os"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

// DeleteArchive preserves linked-server guards. Unlinked archives are purged
// only explicitly; failed cleanup keeps checkpoints for an explicit retry.
func (s *BackupService) DeleteArchive(ctx context.Context, expected database.Backup) error {
	// Never reinterpret a server-authorized request as independent custody.
	if expected.ServerID != "" {
		return s.delete(ctx, expected.ServerID, expected.Name, model.BackupStorageAll, expected.ID)
	}

	id := expected.ID

	release, err := s.acquire("archive:" + id)
	if err != nil {
		return err
	}
	defer release()

	claimed, err := s.Store.ClaimArchiveDeletion(ctx, expected)
	if err != nil {
		return err
	}

	if !claimed {
		return &model.FileError{Code: model.FileConflict, Message: "Unpin the backup before deleting it, and refresh if its association changed."}
	}
	// Refresh after claiming: a previous attempt may already have removed a copy.
	row, err := s.Store.ByID(ctx, id)
	if err != nil {
		return err
	}

	if err := s.deleteRemoteCopy(ctx, &row); err != nil {
		return err
	}

	if row.Local {
		if row.LocalStore == "panel" {
			if row.LocalKey != row.ID+".tar.gz" {
				return repository.ErrAccessInput
			}

			root, err := os.OpenRoot(s.LibraryDirectory)
			if err != nil {
				return err
			}

			err = root.Remove(row.LocalKey)
			_ = root.Close()

			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		} else {
			if _, err := s.localArchiveFiles(ctx, row, model.FileOperationBackupDelete, 0); err != nil {
				return err
			}
		}

		if err := s.Store.CheckpointArchiveCopyDeletion(ctx, row, model.BackupStorageLocal); err != nil {
			return err
		}
	}

	return s.Store.RemoveArchive(ctx, id)
}
