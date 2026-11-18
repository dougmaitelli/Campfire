package service

import (
	"context"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"
)

// deleteRemoteCopy checkpoints confirmed removal before another copy is touched.
// Both linked retention/deletion and independent purge hold their own leases.
// On a checkpoint error the durable intent remains, and no local copy is removed.
func (s *BackupService) deleteRemoteCopy(ctx context.Context, row *database.Backup) error {
	if row.StoreID == "" {
		return nil
	}

	remote, err := s.storage(*row)
	if err != nil {
		return err
	}

	if err := remote.Delete(ctx, row.Key, row.Version); err != nil {
		return err
	}

	if err := s.Store.CheckpointArchiveCopyDeletion(ctx, *row, model.BackupStorageS3); err != nil {
		return err
	}

	row.Remote, row.RemoteDeleting, row.StoreID, row.Version = false, false, "", ""

	return nil
}
