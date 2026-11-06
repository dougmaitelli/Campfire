package runtime

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"campfire/internal/shared/model"
)

func TestUploadCleanupScansPeriodicallyAndExpiresActiveSessions(t *testing.T) {
	m, d := fileTestManager(t)
	directory := filepath.Join(m.root(d.ServerID), "data")
	old := time.Now().Add(-2 * time.Hour)
	staging := func(name string, folder bool) string {
		t.Helper()

		path := filepath.Join(directory, name)
		if folder {
			if err := os.MkdirAll(path, 0700); err != nil {
				t.Fatal(err)
			}
		} else if err := os.WriteFile(path, []byte{1}, 0600); err != nil {
			t.Fatal(err)
		}

		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}

		return path
	}
	first := staging(".campfire-upload-abandoned", false)
	archive := staging(".campfire-archive-abandoned", true)
	active := staging(".campfire-upload-active", false)

	entry, err := m.load(d.ServerID)
	if err != nil {
		t.Fatal(err)
	}

	entry.uploads = map[string]*fileUpload{"active": {temporary: filepath.Base(active), touched: time.Now()}}
	call := func() {
		t.Helper()

		if _, err := m.Files(t.Context(), d, model.FileRequest{ServerID: d.ServerID, Operation: model.FileOperationList, Path: "."}); err != nil {
			t.Fatal(err)
		}
	}

	call()

	for _, path := range []string{first, archive} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("initial discovery did not remove abandoned staging", path, err)
		}
	}

	if _, err := os.Stat(active); err != nil {
		t.Fatal("discovery removed an active upload", err)
	}

	second := staging(".campfire-upload-later", false)
	entry.uploads["active"].touched = old

	call()

	if _, err := os.Stat(second); err != nil {
		t.Fatal("file request repeated the directory scan before its interval", err)
	}

	if _, err := os.Stat(active); !errors.Is(err, os.ErrNotExist) || len(entry.uploads) != 0 {
		t.Fatal("throttled discovery prevented active-session expiry", err)
	}

	entry.lastUploadScan = time.Now().Add(-uploadScanInterval)

	call()

	if _, err := os.Stat(second); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("periodic discovery did not resume", err)
	}
}
