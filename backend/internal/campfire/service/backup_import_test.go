package service

import (
	"os"
	"testing"

	"campfire/internal/shared/model"
)

func TestBackupUploadsAreOwnerBoundCompleteAndCleaned(t *testing.T) {
	imports := NewBackupImports()

	token, err := imports.Begin(1, "server", 4, model.DefaultUploadLimit)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { imports.Cancel(1, token) })

	if err := imports.With(2, token, func(*BackupUpload) error { t.Fatal("another account accessed upload"); return nil }); err == nil {
		t.Fatal("owner boundary missing")
	}

	err = imports.With(1, token, func(upload *BackupUpload) error {
		if _, err := os.Stat(upload.file.Name()); !os.IsNotExist(err) {
			t.Fatal("temporary archive is visible by path", err)
		}

		if err := upload.Chunk(1, []byte("bad")); err == nil {
			t.Fatal("accepted out-of-order chunk")
		}

		if err := upload.Chunk(0, []byte("12345")); err == nil {
			t.Fatal("accepted overflow")
		}

		if err := upload.Inspect(t.Context()); err == nil {
			t.Fatal("accepted incomplete upload")
		}

		if err := upload.Chunk(0, []byte("1234")); err != nil {
			t.Fatal(err)
		}

		if err := upload.Inspect(t.Context()); err == nil {
			t.Fatal("accepted invalid gzip")
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	imports.Cancel(1, token)

	if err := imports.With(1, token, func(*BackupUpload) error { return nil }); err == nil {
		t.Fatal("cancelled upload survived")
	}

	if _, err := imports.Begin(1, "server", 5, 4); err == nil {
		t.Fatal("accepted upload above limit")
	}
}
