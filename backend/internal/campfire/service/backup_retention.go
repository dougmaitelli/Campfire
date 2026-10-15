package service

import (
	"context"
	"errors"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"
)

func expiredBackup(row database.Backup, days int, now time.Time) bool {
	return days > 0 && time.Unix(0, row.CreatedAt).Before(now.Add(-time.Duration(days)*24*time.Hour))
}

func remoteCandidates(rows []database.Backup, p model.BackupPolicy, now time.Time) map[string]bool {
	result := make(map[string]bool)
	index := 0

	for _, row := range rows {
		if row.Deleting || row.NoRemote || (!row.Local && !row.Remote) {
			continue
		}

		if p.Storage == "s3" && (p.RemoteKeepLast == 0 || index < p.RemoteKeepLast) && !expiredBackup(row, p.RemoteMaxAgeDays, now) {
			result[row.Name] = true
		}

		index++
	}

	return result
}

func (s *BackupService) reconcile(ctx context.Context, id string) error {
	if s.Deployment.Transfers != nil {
		if err := s.Deployment.Transfers.Store.Guard(ctx, id); err != nil {
			return err
		}
	}

	discoverErr := s.discover(ctx, id)

	rows, err := s.Store.List(ctx, id)
	if err != nil {
		return err
	}

	for _, row := range rows {
		if row.Deleting || row.LocalDeleting || row.RemoteDeleting {
			if err := s.deleteCopies(ctx, row); err != nil {
				_ = s.Store.Update(ctx, id, row.Name, map[string]any{"error": "Deletion is pending; verify Spark and S3 availability."})
			}
		}
	}

	p, err := s.Store.Policy(ctx, id)
	if err != nil {
		return err
	}

	rows, err = s.Store.List(ctx, id)
	if err != nil {
		return err
	}

	now := time.Now()

	eligible := remoteCandidates(rows, p, now)
	for _, row := range rows {
		if eligible[row.Name] && row.Local && !row.Remote && !row.LocalDeleting && !row.RemoteDeleting {
			if err := s.upload(ctx, row); err != nil {
				_ = s.Store.Update(ctx, id, row.Name, map[string]any{"error": "S3 synchronization failed; the local archive was retained. Background sync will retry."})
			}
		}
	}

	rows, err = s.Store.List(ctx, id)
	if err != nil {
		return err
	}

	p, err = s.Store.Policy(ctx, id)
	if err != nil {
		return err
	}

	eligible = remoteCandidates(rows, p, now)

	var localIndex, remoteIndex int

	for _, row := range rows {
		if row.Deleting || row.LocalDeleting || row.RemoteDeleting {
			continue
		}

		removeLocal, removeRemote := false, false
		if row.Local {
			removeLocal = p.LocalKeepLast > 0 && localIndex >= p.LocalKeepLast || expiredBackup(row, p.LocalMaxAgeDays, now)
			localIndex++

			if eligible[row.Name] && !row.Remote {
				removeLocal = false
			}
		}

		if row.Remote {
			removeRemote = p.RemoteKeepLast > 0 && remoteIndex >= p.RemoteKeepLast || expiredBackup(row, p.RemoteMaxAgeDays, now)
			remoteIndex++
		}

		if removeLocal || removeRemote {
			row.LocalDeleting, row.RemoteDeleting = removeLocal, removeRemote
			if err := s.Store.Update(ctx, id, row.Name, map[string]any{"local_deleting": removeLocal, "remote_deleting": removeRemote}); err != nil {
				return err
			}

			if err := s.deleteCopies(ctx, row); err != nil {
				_ = s.Store.Update(ctx, id, row.Name, map[string]any{"error": "Retention cleanup is pending; verify Spark and S3 availability."})
			}
		}
	}

	return discoverErr
}

// Delete persists intent before deleting either copy. Failed cleanup is retried
// by the background worker without rediscovering and reuploading the archive.
func (s *BackupService) Delete(ctx context.Context, id, name, location string) error {
	if !model.BackupName(name) || (location != "all" && location != "local" && location != "s3") {
		return &model.FileError{Code: "invalid_file", Message: "Select a backup and storage location."}
	}

	release, err := s.acquire(id)
	if err != nil {
		return err
	}
	defer release()

	if s.Deployment.Transfers != nil {
		if err := s.Deployment.Transfers.Store.Guard(ctx, id); err != nil {
			return err
		}
	}

	_ = s.discover(ctx, id)

	row, err := s.Store.Get(ctx, id, name)
	if err != nil {
		return err
	}

	if row.Deleting || row.LocalDeleting || row.RemoteDeleting {
		return &model.FileError{Code: "file_busy", Message: "Backup deletion is already pending."}
	}

	row.Deleting = location == "all"
	row.LocalDeleting = location == "all" || location == "local"

	row.RemoteDeleting = location == "all" || location == "s3"
	if err := s.Store.Update(ctx, id, name, map[string]any{"deleting": row.Deleting, "local_deleting": row.LocalDeleting, "remote_deleting": row.RemoteDeleting, "no_remote": row.NoRemote || row.RemoteDeleting, "error": ""}); err != nil {
		return err
	}

	if err := s.deleteCopies(ctx, row); err != nil {
		_ = s.Store.Update(ctx, id, name, map[string]any{"error": "Deletion is pending; verify Spark and S3 availability."})
	}

	return nil
}

func (s *BackupService) deleteCopies(ctx context.Context, row database.Backup) error {
	release, err := s.Deployment.lockServer(ctx, row.ServerID)
	if err != nil {
		return err
	}
	defer release()

	if s.Deployment.Transfers != nil {
		if err := s.Deployment.Transfers.Store.Guard(ctx, row.ServerID); err != nil {
			return err
		}
	}

	status, statusErr := s.Deployment.runActionUnlocked(ctx, row.ServerID, "status", "")
	if statusErr == nil && status.Busy {
		return &model.FileError{Code: "file_busy", Message: "Wait for the current server operation to finish."}
	}

	if row.RemoteDeleting && row.StoreID != "" {
		remote, err := s.storage(row)
		if err != nil {
			return err
		}

		if err := remote.Delete(ctx, row.Key, row.Version); err != nil {
			return err
		}

		row.Remote = false
	}

	if row.LocalDeleting {
		if statusErr != nil {
			return statusErr
		}

		_, err := s.Deployment.filesUnlocked(ctx, row.ServerID, model.FileRequest{Operation: "backup.delete", Path: row.Name})

		var failure *model.FileError

		missing := errors.As(err, &failure) && failure.Code == "file_not_found"
		if err != nil && !missing {
			return err
		}

		row.Local = false
	}

	if !row.Local && !row.Remote {
		return s.Store.Remove(ctx, row.ServerID, row.Name)
	}

	return s.Store.Update(ctx, row.ServerID, row.Name, map[string]any{"local": row.Local, "remote": row.Remote, "local_deleting": false, "remote_deleting": false, "error": ""})
}

// BeforeServerDelete is called under the placement/lifecycle lock. It reserves
// the backup worker and removes off-host objects before local game deletion.
func (s *BackupService) BeforeServerDelete(ctx context.Context, id string) (func(), error) {
	release, err := s.acquire(id)
	if err != nil {
		return nil, err
	}

	rows, err := s.Store.List(ctx, id)
	if err != nil {
		release()
		return nil, err
	}

	for _, row := range rows {
		if err := s.Store.Update(ctx, id, row.Name, map[string]any{"deleting": true, "local_deleting": true, "remote_deleting": true}); err != nil {
			release()
			return nil, err
		}

		if row.StoreID != "" {
			remote, err := s.storage(row)
			if err != nil {
				release()
				return nil, err
			}

			if err := remote.Delete(ctx, row.Key, row.Version); err != nil {
				release()
				return nil, err
			}

			if err := s.Store.Update(ctx, id, row.Name, map[string]any{"remote": false, "remote_deleting": false}); err != nil {
				release()
				return nil, err
			}
		}
	}

	return release, nil
}

// ForgetServer removes metadata after the runtime has confirmed local cleanup.
func (s *BackupService) ForgetServer(ctx context.Context, id string) error {
	rows, err := s.Store.List(ctx, id)
	if err != nil {
		return err
	}

	for _, row := range rows {
		if err := s.Store.Remove(ctx, id, row.Name); err != nil {
			return err
		}
	}

	return s.Store.ForgetServer(ctx, id)
}
