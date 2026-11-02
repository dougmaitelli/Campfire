package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"campfire/internal/shared/model"
)

func TestBackupUploadUsesStateStorageAndReservesAggregateCapacity(t *testing.T) {
	const size = 65 << 20

	directory := filepath.Join(t.TempDir(), "backup-imports")
	imports := NewBackupImports(directory, size)
	// The deployment's /tmp can be too small or unavailable for large archives.
	t.Setenv("TMPDIR", filepath.Join(directory, "unavailable"))

	token, err := imports.Begin(1, "server", size, model.DefaultUploadLimit)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { imports.Cancel(1, token) })

	if _, err := imports.Begin(2, "other", 1, model.DefaultUploadLimit); err == nil {
		t.Fatal("aggregate staging budget was exceeded")
	}

	err = imports.With(1, token, func(upload *BackupUpload) error {
		if !strings.HasPrefix(upload.file.Name(), directory+string(os.PathSeparator)) {
			t.Fatal("upload was not staged on the configured volume")
		}

		data := make([]byte, model.FileLimit)
		for upload.offset < size {
			if err := upload.Chunk(upload.offset, data[:min(int64(len(data)), size-upload.offset)]); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	imports.Cancel(1, token)

	next, err := imports.Begin(2, "other", size, model.DefaultUploadLimit)
	if err != nil {
		t.Fatal("cancellation did not release staging capacity", err)
	}

	imports.Cancel(2, next)

	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 0 {
		t.Fatal("staging retained credential-bearing archives", err)
	}
}

func TestBackupUploadsAreOwnerBoundCompleteAndCleaned(t *testing.T) {
	imports := NewBackupImports(t.TempDir(), model.DefaultUploadLimit)

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
