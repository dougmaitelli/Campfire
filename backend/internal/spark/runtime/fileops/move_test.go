package fileops

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"campfire/internal/shared/model"
)

func TestMoveNeverReplacesExistingFilesOrFolders(t *testing.T) {
	for _, directory := range []bool{false, true} {
		t.Run(map[bool]string{false: "file", true: "directory"}[directory], func(t *testing.T) {
			volume := t.TempDir()

			m := New(Host{})
			defer m.Close()

			for _, name := range []string{"source", "destination"} {
				filename := filepath.Join(volume, name)
				if directory {
					if err := os.Mkdir(filename, 0700); err != nil {
						t.Fatal(err)
					}

					filename = filepath.Join(filename, "contents")
				}

				if err := os.WriteFile(filename, []byte(name), 0600); err != nil {
					t.Fatal(err)
				}
			}

			q := model.FileRequest{ServerID: "server", Operation: model.FileOperationRename, Path: "source", Destination: "destination"}
			_, err := m.Files(t.Context(), model.Deployment{ServerID: "server"}, volume, &State{}, q)

			var fileErr *model.FileError
			if !errors.As(err, &fileErr) || fileErr.Code != model.FileConflict {
				t.Fatal("expected a conflict", err)
			}

			for _, name := range []string{"source", "destination"} {
				filename := filepath.Join(volume, name)
				if directory {
					filename = filepath.Join(filename, "contents")
				}

				data, err := os.ReadFile(filename)
				if err != nil || string(data) != name {
					t.Fatal("collision changed content", name, string(data), err)
				}
			}

			q.Destination = "new-destination"
			if _, err := m.Files(t.Context(), model.Deployment{ServerID: "server"}, volume, &State{}, q); err != nil {
				t.Fatal(err)
			}

			if _, err := os.Stat(filepath.Join(volume, "source")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("source was not moved", err)
			}
		})
	}
}

func TestMovePublicationRejectsArrivalAfterPreflight(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()

	if err := os.WriteFile(filepath.Join(root.Name(), "source"), []byte("source"), 0600); err != nil {
		t.Fatal(err)
	}

	if _, err := root.Lstat("destination"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}

	// A writer outside the panel may create the destination after preflight.
	if err := os.WriteFile(filepath.Join(root.Name(), "destination"), []byte("arrival"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := RenameNoReplace(root, "source", "destination"); !errors.Is(err, os.ErrExist) {
		t.Fatal("overwrote a concurrent arrival", err)
	}

	for name, want := range map[string]string{"source": "source", "destination": "arrival"} {
		data, err := root.ReadFile(name)
		if err != nil || string(data) != want {
			t.Fatal(name, string(data), err)
		}
	}
}

func TestMoveRejectsFolderIntoItself(t *testing.T) {
	volume := t.TempDir()
	if err := os.MkdirAll(filepath.Join(volume, "source", "child"), 0700); err != nil {
		t.Fatal(err)
	}

	m := New(Host{})
	defer m.Close()

	q := model.FileRequest{ServerID: "server", Operation: model.FileOperationRename, Path: "source", Destination: "source/child/moved"}
	_, err := m.Files(t.Context(), model.Deployment{ServerID: "server"}, volume, &State{}, q)

	var fileErr *model.FileError
	if !errors.As(err, &fileErr) || fileErr.Code != model.FileInvalid {
		t.Fatal("accepted descendant destination", err)
	}
}
