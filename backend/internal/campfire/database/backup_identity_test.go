package database_test

import (
	"path/filepath"
	"strings"
	"testing"

	"campfire/internal/campfire/database"
)

func TestBackupIdentityConstraintsFailClosed(t *testing.T) {
	source := filepath.Join(t.TempDir(), "panel.db")

	db, err := database.Open(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}
	// Keep every column but remove the identity constraints. Merely seeing an
	// `id` column must not cause startup to accept a malformed archive catalog.
	for _, query := range []string{"CREATE TABLE malformed_backups AS SELECT * FROM backups", "DROP TABLE backups", "ALTER TABLE malformed_backups RENAME TO backups"} {
		if err := db.ORM().Exec(query).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := database.Open(t.Context(), source)
	if err == nil {
		_ = reopened.Close()

		t.Fatal("accepted missing primary key")
	}

	if !strings.Contains(err.Error(), "backup identity") {
		t.Fatal(err)
	}
}
