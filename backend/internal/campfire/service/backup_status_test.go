package service

import (
	"context"
	"io"
	"testing"
	"time"

	"campfire/internal/shared/model"
)

type pausedBackupStorage struct {
	BackupStorage
	started chan struct{}
	resume  chan struct{}
}

func (s *pausedBackupStorage) Put(ctx context.Context, key string, size int64, digest string, input io.Reader) (string, error) {
	close(s.started)

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-s.resume:
		return s.BackupStorage.Put(ctx, key, size, digest, input)
	}
}

func assertBackupStates(t *testing.T, service *BackupService, id string, expected map[string]model.BackupState) {
	t.Helper()

	items, err := service.List(t.Context(), id)
	if err != nil || len(items) != len(expected) {
		t.Fatal(items, err)
	}

	for _, item := range items {
		if item.State != expected[item.Name] {
			t.Fatalf("server list: %s has state %s, want %s", item.Name, item.State, expected[item.Name])
		}
	}

	catalog, err := service.Catalog(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	for _, item := range catalog {
		if item.ServerID == id && item.State != expected[item.Name] {
			t.Fatalf("catalog: %s has state %s, want %s", item.Name, item.State, expected[item.Name])
		}
	}
}

func TestBackupDiscoveryDoesNotMarkLocalArchivesSyncing(t *testing.T) {
	f, service, _, _ := backupFixture(t)
	seedBackup(t, f, "snapshot", time.Now())

	if err := service.discover(t.Context(), f.server.ID); err != nil {
		t.Fatal(err)
	}

	for range 3 {
		release, err := service.acquire(f.server.ID)
		if err != nil {
			t.Fatal(err)
		}

		assertBackupStates(t, service, f.server.ID, map[string]model.BackupState{"snapshot.tar.gz": model.BackupAvailable})
		release()
		assertBackupStates(t, service, f.server.ID, map[string]model.BackupState{"snapshot.tar.gz": model.BackupAvailable})
	}
}

func TestBackupSyncingAppliesOnlyToCurrentUpload(t *testing.T) {
	for _, failed := range []bool{false, true} {
		name := "completed"
		if failed {
			name = "failed"
		}

		t.Run(name, func(t *testing.T) {
			f, service, storage, _ := backupFixture(t)
			seedBackup(t, f, "older", time.Now().Add(-time.Hour))
			seedBackup(t, f, "uploading", time.Now())

			storage.fail = failed
			remote := &pausedBackupStorage{BackupStorage: storage, started: make(chan struct{}), resume: make(chan struct{})}

			service.Remote = remote
			if err := service.SavePolicy(t.Context(), f.server.ID, model.BackupPolicy{Storage: model.BackupStorageS3, RemoteKeepLast: 1}); err != nil {
				t.Fatal(err)
			}

			release, err := service.acquire(f.server.ID)
			if err != nil {
				t.Fatal(err)
			}

			ctx, cancel := context.WithCancel(t.Context())

			finished, result := make(chan struct{}), make(chan error, 1)
			go func() {
				defer close(finished)
				defer release()

				result <- service.reconcile(ctx, f.server.ID)
			}()

			defer func() {
				cancel()

				<-finished
			}()

			select {
			case <-remote.started:
			case <-time.After(5 * time.Second):
				t.Fatal("backup upload did not start")
			}

			for range 3 {
				assertBackupStates(t, service, f.server.ID, map[string]model.BackupState{
					"older.tar.gz": model.BackupAvailable, "uploading.tar.gz": model.BackupSyncing,
				})
			}

			for _, item := range []struct {
				name  string
				state model.BackupState
			}{{"older.tar.gz", model.BackupAvailable}, {"uploading.tar.gz", model.BackupSyncing}} {
				updated, err := service.Update(t.Context(), f.server.ID, item.name, model.BackupChanges{DisplayName: new("During upload")})
				if err != nil || updated.State != item.state || updated.DisplayName != "During upload" {
					t.Fatal("metadata update changed upload status", updated, err)
				}
			}

			close(remote.resume)

			if err := <-result; err != nil {
				t.Fatal(err)
			}

			<-finished

			state := model.BackupAvailable
			if failed {
				state = model.BackupError
			}

			assertBackupStates(t, service, f.server.ID, map[string]model.BackupState{
				"older.tar.gz": model.BackupAvailable, "uploading.tar.gz": state,
			})
		})
	}
}
