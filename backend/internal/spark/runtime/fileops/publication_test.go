package fileops

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestSharedPublicationRejectsNonregularStages(t *testing.T) {
	for _, kind := range []string{"directory", "symlink", "fifo"} {
		t.Run(kind, func(t *testing.T) {
			root, err := os.OpenRoot(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = root.Close() }()

			if err := os.WriteFile(filepath.Join(root.Name(), "target.txt"), []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}

			switch kind {
			case "directory":
				err = root.Mkdir("staged", 0700)
			case "symlink":
				err = root.Symlink("target.txt", "staged")
			case "fifo":
				err = syscall.Mkfifo(filepath.Join(root.Name(), "staged"), 0600)
			}

			if err != nil {
				t.Fatal(err)
			}

			info, err := root.Stat("target.txt")
			if err != nil {
				t.Fatal(err)
			}

			if err := PublishStagedFile(t.Context(), root, "staged", "target.txt", StatRevision(info)); err == nil {
				t.Fatal("published a nonregular staged entry")
			}

			data, err := root.ReadFile("target.txt")
			if err != nil || string(data) != "original" {
				t.Fatal("changed destination", string(data), err)
			}
		})
	}
}

func TestDestinationRevisionDetectsSameSizeReplacementWithRestoredMtime(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()

	for name, data := range map[string]string{"staged": "new", "target.txt": "old"} {
		if err := os.WriteFile(filepath.Join(root.Name(), name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}

	before, err := root.Stat("target.txt")
	if err != nil {
		t.Fatal(err)
	}

	if err := root.Remove("target.txt"); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(root.Name(), "target.txt"), []byte("own"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := os.Chtimes(filepath.Join(root.Name(), "target.txt"), before.ModTime(), before.ModTime()); err != nil {
		t.Fatal(err)
	}

	if err := PublishStagedFile(t.Context(), root, "staged", "target.txt", StatRevision(before)); err == nil {
		t.Fatal("replaced a different inode with the same size and mtime")
	}

	data, err := root.ReadFile("target.txt")
	if err != nil || string(data) != "own" {
		t.Fatal("replaced concurrent content", string(data), err)
	}
}
