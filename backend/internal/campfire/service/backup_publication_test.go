package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/backuparchive"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

func TestStoredArchiveRegistrationFinishesAfterBrowserCancellation(t *testing.T) {
	f, backups, _, _ := backupFixture(t)
	imports := NewBackupImports(t.TempDir(), model.DefaultUploadLimit)
	backups.LibraryDirectory = imports.ArchiveDirectory()
	payload := []byte("already validated portable archive")

	token, err := imports.Begin(1, "", int64(len(payload)), model.DefaultUploadLimit)
	if err != nil {
		t.Fatal(err)
	}
	defer imports.Cancel(1, token)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	// Cancel precisely after durable publication, when database registration begins.
	if err := f.db.ORM().Callback().Create().Before("gorm:create").Register("cancel-browser", func(tx *gorm.DB) {
		if tx.Statement.Table == "backups" {
			cancel()
		}
	}); err != nil {
		t.Fatal(err)
	}

	var stored database.Backup

	err = imports.With(1, token, func(upload *BackupUpload) error {
		if err := upload.Chunk(0, payload); err != nil {
			return err
		}
		// This test isolates finalization; inspection is covered by import API tests.
		upload.Name, upload.Manifest.Mode = "full-test.tar.gz", model.BackupModeFull
		upload.Manifest.Format = backuparchive.Format

		var err error

		stored, err = upload.Store(ctx, backups)

		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	if ctx.Err() == nil {
		t.Fatal("did not cancel caller")
	}

	imports.Cancel(1, token)

	if _, err := backups.Store.ByID(t.Context(), stored.ID); err != nil {
		t.Fatal("stored bytes lost catalog registration", err)
	}

	data, err := os.ReadFile(filepath.Join(backups.LibraryDirectory, stored.LocalKey))
	if err != nil || string(data) != string(payload) {
		t.Fatalf("cancelled receipt lost archive: %q %v", data, err)
	}

	entries, err := os.ReadDir(filepath.Join(imports.directory, "uploads"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("finalized upload retained temporary buffers: %v %v", entries, err)
	}
}
