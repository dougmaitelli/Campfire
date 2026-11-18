package upload

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPublicationIsDurableIdempotentAndDoesNotReplace(t *testing.T) {
	directory := t.TempDir()

	session, err := Create(directory, "upload-*", 3, 3)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	if err := session.Write(0, []byte("new")); err != nil {
		t.Fatal(err)
	}

	destination := filepath.Join(directory, "archive.tar.gz")
	if err := os.WriteFile(destination, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := session.PublishNoReplace(destination); !errors.Is(err, os.ErrExist) {
		t.Fatalf("replaced existing archive: %v", err)
	}

	data, err := os.ReadFile(destination)
	if err != nil || string(data) != "old" {
		t.Fatalf("existing archive changed: %q %v", data, err)
	}

	destination = filepath.Join(directory, "new.tar.gz")
	for range 2 {
		if err := session.PublishNoReplace(destination); err != nil {
			t.Fatal(err)
		}
	}

	temporary := session.File.Name()
	session.Close()

	if _, err := os.Stat(temporary); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary buffer remains: %v", err)
	}

	data, err = os.ReadFile(destination)
	if err != nil || string(data) != "new" {
		t.Fatalf("archive did not survive session close: %q %v", data, err)
	}
}

func TestIncompleteUploadCannotBePublished(t *testing.T) {
	directory := t.TempDir()

	session, err := Create(directory, "upload-*", 3, 3)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	if err := session.Write(0, []byte("a")); err != nil {
		t.Fatal(err)
	}

	destination := filepath.Join(directory, "archive.tar.gz")
	if err := session.PublishNoReplace(destination); err == nil {
		t.Fatal("published incomplete upload")
	}

	if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("created incomplete archive: %v", err)
	}
}
