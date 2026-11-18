package database_test

import (
	"os"
	"path/filepath"
	"testing"

	"campfire/internal/campfire/database"
)

func TestServerDeletionMigrationPreservesIndependentArchiveIdentity(t *testing.T) {
	source := filepath.Join(t.TempDir(), "panel.db")
	if configured := os.Getenv("CAMPFIRE_TEST_MIGRATION_POSTGRES"); configured != "" {
		source = configured
	}

	db, err := database.Open(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}

	archive := database.Backup{ID: "existing-independent-id", Name: "full-snapshot.tar.gz", Local: true, LocalStore: "panel", LocalKey: "existing-independent-id.tar.gz", Key: "unchanged-object-key", Pinned: true}
	if err := db.ORM().Create(&archive).Error; err != nil {
		t.Fatal(err)
	}

	for _, query := range []string{"ALTER TABLE servers DROP COLUMN deletion_pending", "UPDATE schema_migrations SET version = 3"} {
		if err := db.ORM().Exec(query).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := database.Open(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()

	if !reopened.ORM().Migrator().HasColumn(&database.Server{}, "deletion_pending") {
		t.Fatal("missing deletion intent")
	}

	var saved database.Backup
	if err := reopened.ORM().Where("id = ?", archive.ID).Take(&saved).Error; err != nil || saved.LocalKey != archive.LocalKey || saved.Key != archive.Key || !saved.Pinned || saved.ServerID != "" {
		t.Fatal("archive changed during lifecycle migration", saved, err)
	}

	if err := reopened.ORM().Delete(&saved).Error; err != nil {
		t.Fatal(err)
	}
}
