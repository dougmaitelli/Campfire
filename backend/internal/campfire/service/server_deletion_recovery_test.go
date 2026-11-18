package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

type lostDeleteResponse struct{ *backupTestRouter }

func (r lostDeleteResponse) Action(ctx context.Context, spark string, request model.RuntimeRequest) (model.RuntimeStatus, error) {
	status, err := r.backupTestRouter.Action(ctx, spark, request)
	if err == nil && request.Action == model.ServerActionDelete {
		return model.RuntimeStatus{}, errors.New("delete response lost")
	}

	return status, err
}

type failedDeletePreparation struct{ *backupTestRouter }

func (r failedDeletePreparation) Deploy(context.Context, string, model.Deployment) (model.ConfigurationReceipt, error) {
	return model.ConfigurationReceipt{}, errors.New("configuration delivery failed")
}

func TestDeletionPreparationFailureDoesNotFenceAnUndispatchedServer(t *testing.T) {
	f, backups, _, router := backupFixture(t)
	backups.Deployment.delivered = nil

	backups.Deployment.Remote = failedDeletePreparation{router}
	if err := backups.Deployment.Delete(t.Context(), f.server.ID); err == nil {
		t.Fatal("expected preparation failure")
	}

	pending, err := repository.NewServerRepository(f.db.ORM()).DeletionPending(t.Context(), f.server.ID)
	if err != nil || pending {
		t.Fatal("undispatched server cannot be corrected", pending, err)
	}

	if err := backups.Deployment.guardServer(t.Context(), f.server.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(f.source.Directory, f.server.ID, "data", "world.bin")); err != nil {
		t.Fatal("preparation deleted game data", err)
	}
}

func TestPendingServerDeletionFencesRetentionAcrossFailureAndRestart(t *testing.T) {
	for _, failure := range []string{"database", "lost-response"} {
		t.Run(failure, func(t *testing.T) {
			f, backups, remote, router := backupFixture(t)
			seedBackup(t, f, "old", time.Now().Add(-2*time.Hour))
			seedBackup(t, f, "new", time.Now())

			policy := model.BackupPolicy{Storage: model.BackupStorageS3}
			if err := backups.SavePolicy(t.Context(), f.server.ID, policy); err != nil {
				t.Fatal(err)
			}

			if err := backups.reconcile(t.Context(), f.server.ID); err != nil {
				t.Fatal(err)
			}

			policy.LocalKeepLast, policy.RemoteKeepLast = 1, 1
			if err := backups.SavePolicy(t.Context(), f.server.ID, policy); err != nil {
				t.Fatal(err)
			}

			if failure == "database" {
				if err := f.db.ORM().Callback().Delete().Before("gorm:delete").Register("fail-server-delete", func(tx *gorm.DB) {
					if tx.Statement.Table == "servers" {
						_ = tx.AddError(errors.New("database deletion failed"))
					}
				}); err != nil {
					t.Fatal(err)
				}
			} else {
				backups.Deployment.Remote = lostDeleteResponse{router}
			}

			if err := backups.Deployment.Delete(t.Context(), f.server.ID); err == nil {
				t.Fatal("expected deletion failure")
			}

			var server database.Server
			if err := f.db.ORM().Where("id = ?", f.server.ID).Take(&server).Error; err != nil || !server.DeletionPending {
				t.Fatal("deletion intent was not durable", server, err)
			}
			// Fresh service instances have no old lifecycle/backup locks or caches.
			original := backups.Deployment
			restarted := &DeploymentService{Servers: NewServerService(repository.NewServerRepository(f.db.ORM()), original.Embers), Sparks: original.Sparks, Embers: original.Embers, Remote: router}

			restarted.Backups = NewBackupService(repository.NewBackupRepository(f.db.ORM()), restarted, remote)
			if err := restarted.Backups.reconcile(t.Context(), f.server.ID); !errors.Is(err, repository.ErrServerDeleting) {
				t.Fatal("retention did not honor pending deletion", err)
			}

			if _, err := restarted.Action(t.Context(), f.server.ID, model.ServerActionStart); !errors.Is(err, repository.ErrServerDeleting) {
				t.Fatal("allowed mutation while deletion pending", err)
			}

			old := filepath.Join(f.source.Directory, f.server.ID, "backups", "old.tar.gz")
			if _, err := os.Stat(old); err != nil || len(remote.objects) != 2 {
				t.Fatal("pending deletion lost recovery copies", err, len(remote.objects))
			}

			if failure == "database" {
				if err := f.db.ORM().Callback().Delete().Remove("fail-server-delete"); err != nil {
					t.Fatal(err)
				}
			}

			deadline := time.Now().Add(5 * time.Second)

			for {
				status, err := router.Action(t.Context(), f.server.SparkID, model.RuntimeRequest{ServerID: f.server.ID, Action: model.ServerActionStatus})
				if err != nil {
					t.Fatal(err)
				}

				if !status.Busy {
					break
				}

				if time.Now().After(deadline) {
					t.Fatal("runtime deletion did not finish")
				}

				time.Sleep(10 * time.Millisecond)
			}

			if err := restarted.Delete(t.Context(), f.server.ID); err != nil {
				t.Fatal("deletion retry failed", err)
			}

			catalog, err := restarted.Backups.Catalog(t.Context())
			if err != nil || len(catalog) != 2 || len(remote.objects) != 2 {
				t.Fatal(catalog, err)
			}

			for _, archive := range catalog {
				if archive.ServerID != "" || !archive.Local || !archive.Remote {
					t.Fatal("retry did not preserve independent copies", archive)
				}
			}

			if _, err := os.Stat(old); err != nil {
				t.Fatal(err)
			}
		})
	}
}
