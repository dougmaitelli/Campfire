package upload

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSequentialUploadsRejectGapsAndCleanTemporaryStorage(t *testing.T) {
	dir := t.TempDir()

	session, err := Create(dir, "upload-", 4, 8)
	if err != nil {
		t.Fatal(err)
	}

	name := session.File.Name()
	if err := session.Write(1, []byte("x")); err == nil {
		t.Fatal("accepted a gap")
	}

	if err := session.Write(0, []byte("12")); err != nil {
		t.Fatal(err)
	}

	if err := session.Complete(); err == nil {
		t.Fatal("incomplete upload accepted")
	}

	if err := session.Write(2, []byte("345")); err == nil {
		t.Fatal("overflow accepted")
	}

	if err := session.Write(2, []byte("34")); err != nil {
		t.Fatal(err)
	}

	if err := session.Complete(); err != nil {
		t.Fatal(err)
	}

	session.Close()
	session.Close()

	if _, err := os.Stat(name); !os.IsNotExist(err) {
		t.Fatal("temporary upload leaked", err)
	}
}

func TestRandomAccessCoverageAndPublishedFileLifetime(t *testing.T) {
	dir := t.TempDir()

	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = root.Close() }()

	session, err := CreateAt(root, "upload-test", 4, 8)
	if err != nil {
		t.Fatal(err)
	}

	if err := session.Write(2, []byte("34")); err != nil {
		t.Fatal(err)
	}

	if err := session.Complete(); err == nil {
		t.Fatal("sparse known-size upload accepted")
	}

	if err := session.Write(0, []byte("12")); err != nil {
		t.Fatal(err)
	}

	if err := session.Complete(); err != nil {
		t.Fatal(err)
	}

	if err := root.Rename("upload-test", "published"); err != nil {
		t.Fatal(err)
	}

	session.Close()

	data, err := os.ReadFile(filepath.Join(dir, "published"))
	if err != nil || string(data) != "1234" {
		t.Fatal("cleanup removed published content", err)
	}
}

func TestPruneOnlyRemovesExpiredInactiveUploads(t *testing.T) {
	dir := t.TempDir()
	old := time.Now().Add(-2 * time.Hour)

	for _, name := range []string{"upload-old", "upload-active", "archive.tar.gz", "upload-new"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("kept"), 0600); err != nil {
			t.Fatal(err)
		}

		if name != "upload-new" {
			if err := os.Chtimes(path, old, old); err != nil {
				t.Fatal(err)
			}
		}
	}

	Prune(dir, "upload-", time.Now().Add(-time.Hour), map[string]bool{filepath.Join(dir, "upload-active"): true})

	for _, name := range []string{"upload-active", "archive.tar.gz", "upload-new"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatal("prune removed retained content", name, err)
		}
	}

	if _, err := os.Stat(filepath.Join(dir, "upload-old")); !os.IsNotExist(err) {
		t.Fatal("old upload retained", err)
	}
}
