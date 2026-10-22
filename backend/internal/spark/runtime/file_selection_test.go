package runtime

import (
	"testing"

	"campfire/internal/shared/model"
)

func TestSelectedArchivePreservesNamesAndExcludesUnselectedFiles(t *testing.T) {
	m, d := fileTestManager(t)
	call := func(q model.FileRequest) model.FileResult {
		t.Helper()

		q.ServerID = d.ServerID

		result, err := m.Files(t.Context(), d, q)
		if err != nil {
			t.Fatal(q, err)
		}

		return result
	}
	call(model.FileRequest{Operation: "mkdir", Path: "config"})
	call(model.FileRequest{Operation: "mkdir", Path: "config/world"})

	for _, name := range []string{"config/settings.txt", "config/world/save.txt", "config/ignored.txt"} {
		call(model.FileRequest{Operation: "write", Path: name, Revision: "absent", Data: []byte(name)})
	}

	call(model.FileRequest{Operation: "compress", Path: "config", Paths: []string{"config/settings.txt", "config/world"}, Destination: "config/selected.tar.gz"})
	call(model.FileRequest{Operation: "extract", Path: "config/selected.tar.gz", Destination: "restored"})

	for _, name := range []string{"settings.txt", "world/save.txt"} {
		read := call(model.FileRequest{Operation: "read", Path: "restored/" + name})
		if string(read.Data) != "config/"+name {
			t.Fatal("selected archive changed the file hierarchy", name)
		}
	}

	listing := call(model.FileRequest{Operation: "list", Path: "restored"})
	if len(listing.Entries) != 2 {
		t.Fatal("archive included unselected files", listing.Entries)
	}
}

func TestSelectedArchiveRejectsUnsafeSourcesAndDestinations(t *testing.T) {
	m, d := fileTestManager(t)
	for _, q := range []model.FileRequest{
		{Operation: "compress", Path: ".", Paths: []string{"../outside"}, Destination: "bad.tar.gz"},
		{Operation: "compress", Path: ".", Paths: []string{"nested/file"}, Destination: "bad.tar.gz"},
		{Operation: "compress", Path: ".", Paths: []string{"same", "same"}, Destination: "bad.tar.gz"},
		{Operation: "compress", Path: ".", Paths: []string{"folder"}, Destination: "folder/bad.tar.gz"},
	} {
		q.ServerID = d.ServerID
		if _, err := m.Files(t.Context(), d, q); err == nil {
			t.Fatal("unsafe selected archive accepted", q)
		}
	}
}
