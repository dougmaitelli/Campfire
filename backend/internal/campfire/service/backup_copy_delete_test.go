package service

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

type failedLocalCopyRemoval struct{ *backupTestRouter }

func (r failedLocalCopyRemoval) Files(ctx context.Context, spark string, q model.FileRequest) (model.FileResult, error) {
	if q.Operation == model.FileOperationBackupDelete {
		return model.FileResult{}, errors.New("local deletion unavailable")
	}

	return r.backupTestRouter.Files(ctx, spark, q)
}

func TestPartialPurgeThenServerDeletionPreservesReadableLocalCopy(t *testing.T) {
	for _, checkpointFails := range []bool{false, true} {
		t.Run(map[bool]string{false: "confirmed", true: "unconfirmed-checkpoint"}[checkpointFails], func(t *testing.T) {
			f, backups, remote, router := backupFixture(t)
			seedBackup(t, f, "snapshot", time.Now())

			if err := backups.SavePolicy(t.Context(), f.server.ID, model.BackupPolicy{Storage: model.BackupStorageS3}); err != nil {
				t.Fatal(err)
			}

			if err := backups.reconcile(t.Context(), f.server.ID); err != nil {
				t.Fatal(err)
			}

			rows, err := backups.Store.List(t.Context(), f.server.ID)
			if err != nil || len(rows) != 1 {
				t.Fatal(rows, err)
			}

			id := rows[0].ID

			if checkpointFails {
				if err := f.db.ORM().Callback().Update().Before("gorm:update").Register("fail-copy-checkpoint", func(tx *gorm.DB) {
					fields, ok := tx.Statement.Dest.(map[string]any)
					if tx.Statement.Table == "backups" && ok && fields["remote"] == false {
						_ = tx.AddError(errors.New("checkpoint unavailable"))
					}
				}); err != nil {
					t.Fatal(err)
				}
			}

			backups.Deployment.Remote = failedLocalCopyRemoval{router}
			if err := backups.Delete(t.Context(), f.server.ID, rows[0].Name, model.BackupStorageAll); err != nil {
				t.Fatal(err)
			}

			if len(remote.objects) != 0 {
				t.Fatal("remote deletion did not execute")
			}

			if checkpointFails {
				var failure *model.FileError
				if err := backups.Deployment.Delete(t.Context(), f.server.ID); !errors.As(err, &failure) || failure.Code != model.FileBusy {
					t.Fatal("detached an unconfirmed removal", err)
				}

				pending, err := repository.NewServerRepository(f.db.ORM()).DeletionPending(t.Context(), f.server.ID)
				if err != nil || pending {
					t.Fatal("fenced the cleanup required for recovery", pending, err)
				}

				if err := f.db.ORM().Callback().Update().Remove("fail-copy-checkpoint"); err != nil {
					t.Fatal(err)
				}

				if err := backups.reconcile(t.Context(), f.server.ID); err != nil {
					t.Fatal("could not settle checkpoint", err)
				}
			}

			row, err := backups.Store.ByID(t.Context(), id)
			if err != nil || row.Remote || row.RemoteDeleting || row.StoreID != "" || row.Version != "" || !row.Local {
				t.Fatal("incorrect checkpoint", row, err)
			}

			if err := backups.Deployment.Delete(t.Context(), f.server.ID); err != nil {
				t.Fatal(err)
			}

			row, err = backups.Store.ByID(t.Context(), id)
			if err != nil || row.ServerID != "" || row.Remote || !row.Local || row.Deleting {
				t.Fatal(row, err)
			}

			result, err := backups.ReadArchive(t.Context(), id, 0)
			if err != nil || len(result.Data) == 0 || !bytes.Equal(result.Data, f.world[:len(result.Data)]) {
				t.Fatal("remaining copy unreadable", err)
			}
		})
	}
}
