package repository_test

import (
	"errors"
	"testing"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
)

func TestBackupPinAndDeletionClaimsAreMutuallyExclusive(t *testing.T) {
	db, err := database.Open(t.Context(), testDatabaseSource(t))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	row := database.Backup{ServerID: "server", Name: "snapshot.tar.gz", Local: true, Key: "immutable-key", Digest: "immutable-digest"}
	store := repository.NewBackupRepository(db.ORM())

	if err := store.Discover(t.Context(), row); err != nil {
		t.Fatal(err)
	}

	if err := store.UpdateMetadata(t.Context(), row.ServerID, row.Name, map[string]any{"pinned": true, "display_name": "Before update"}); err != nil {
		t.Fatal(err)
	}

	if claimed, err := store.BeginDeletion(t.Context(), row.ServerID, row.Name, repository.BackupDeletion{Local: true}); err != nil || claimed {
		t.Fatal("stale retention scan claimed a pinned backup", claimed, err)
	}

	if err := store.Discover(t.Context(), row); err != nil {
		t.Fatal(err)
	}

	kept, err := store.Get(t.Context(), row.ServerID, row.Name)
	if err != nil || !kept.Pinned || kept.DisplayName != "Before update" || kept.Key != row.Key || kept.Digest != row.Digest {
		t.Fatal("discovery changed snapshot metadata or archive identity", kept, err)
	}

	if err := store.UpdateMetadata(t.Context(), row.ServerID, row.Name, map[string]any{"pinned": false}); err != nil {
		t.Fatal(err)
	}

	if claimed, err := store.BeginDeletion(t.Context(), row.ServerID, row.Name, repository.BackupDeletion{Local: true}); err != nil || !claimed {
		t.Fatal("unpinned backup could not be claimed", claimed, err)
	}

	if err := store.UpdateMetadata(t.Context(), row.ServerID, row.Name, map[string]any{"pinned": true}); !errors.Is(err, repository.ErrAccessConflict) {
		t.Fatal("pin acknowledged after deletion began", err)
	}
}
