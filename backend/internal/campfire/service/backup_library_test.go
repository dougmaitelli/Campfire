package service

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

func TestIndependentArchivePurgeCheckpointsOfflineCopies(t *testing.T) {
	f, s, remote, router := backupFixture(t)
	seedBackup(t, f, "snapshot", time.Now())

	if err := s.SavePolicy(t.Context(), f.server.ID, model.BackupPolicy{Storage: model.BackupStorageS3}); err != nil {
		t.Fatal(err)
	}

	if err := s.reconcile(t.Context(), f.server.ID); err != nil {
		t.Fatal(err)
	}

	row, err := s.Store.Get(t.Context(), f.server.ID, "snapshot.tar.gz")
	if err != nil {
		t.Fatal(err)
	}

	if err := s.Deployment.Delete(t.Context(), f.server.ID); err != nil {
		t.Fatal(err)
	}

	row, err = s.Store.ByID(t.Context(), row.ID)
	if err != nil {
		t.Fatal(err)
	}

	router.offline = true

	result, err := s.ReadArchive(t.Context(), row.ID, 0)
	if err != nil || len(result.Data) == 0 || !bytes.Equal(result.Data, f.world[:len(result.Data)]) {
		t.Fatal("offline Spark hid S3 archive", err)
	}

	if err := s.DeleteArchive(t.Context(), row); err == nil {
		t.Fatal("offline local purge succeeded")
	}

	partial, err := s.Store.ByID(t.Context(), row.ID)
	if err != nil || !partial.Local || partial.Remote || !partial.Deleting || len(remote.objects) != 0 {
		t.Fatal("partial purge checkpoint", partial, err)
	}
	// Unlinked archives never re-enter server retention, including deletion retries.
	_ = s.reconcile(t.Context(), f.server.ID)
	if _, err := s.Store.ByID(t.Context(), row.ID); err != nil {
		t.Fatal("orphan cleanup removed metadata", err)
	}

	router.offline = false

	if err := s.DeleteArchive(t.Context(), row); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Store.ByID(t.Context(), row.ID); !errors.Is(err, repository.ErrAccessNotFound) {
		t.Fatal("purged catalog row remains", err)
	}

	if _, err := os.Stat(filepath.Join(f.source.Directory, f.server.ID, "backups", "snapshot.tar.gz")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("purged local bytes remain", err)
	}
}

func TestServerDeletionInventoriesUnknownLocalBackupsAndFailsClosedOffline(t *testing.T) {
	f, s, _, router := backupFixture(t)
	seedBackup(t, f, "uncataloged", time.Now())

	router.offline = true

	if err := s.Deployment.Delete(t.Context(), f.server.ID); err == nil {
		t.Fatal("offline deletion succeeded")
	}

	if _, err := s.Deployment.Servers.Get(t.Context(), f.server.ID); err != nil {
		t.Fatal("failed deletion forgot server", err)
	}

	router.offline = false

	if err := s.Deployment.Delete(t.Context(), f.server.ID); err != nil {
		t.Fatal(err)
	}

	catalog, err := s.Catalog(t.Context())
	if err != nil || len(catalog) != 1 || catalog[0].ServerID != "" {
		t.Fatal("uncataloged backup not detached", catalog, err)
	}

	result, err := s.ReadArchive(t.Context(), catalog[0].ID, 0)
	if err != nil || len(result.Data) == 0 || !bytes.Equal(result.Data, f.world[:len(result.Data)]) {
		t.Fatal("local archive not recoverable", err)
	}
}

func TestDeletingServerLeavesExistingIndependentArchivesUntouched(t *testing.T) {
	f, s, remote, _ := backupFixture(t)

	orphan := database.Backup{ID: "orphan", Name: "orphan.tar.gz", Mode: model.BackupModeFull, Local: true, Remote: true, Pinned: true, Key: "opaque-key", StoreID: remote.ID(), Size: 4, CreatedAt: time.Now().UnixNano()}
	if err := s.Store.SaveArchive(t.Context(), orphan); err != nil {
		t.Fatal(err)
	}

	remote.objects[orphan.Key] = []byte("kept")

	if err := s.Deployment.Delete(t.Context(), f.server.ID); err != nil {
		t.Fatal(err)
	}

	after, err := s.Store.ByID(t.Context(), orphan.ID)
	if err != nil || after != orphan || !bytes.Equal(remote.objects[orphan.Key], []byte("kept")) {
		t.Fatal("independent archive changed", after, err)
	}
}
