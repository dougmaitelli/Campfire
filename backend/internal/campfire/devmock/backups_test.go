//go:build !production

package devmock

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"testing"

	"campfire/internal/shared/model"
)

func TestMockBackupsPersistDownloadAndRestore(t *testing.T) {
	runtime, db, auth, servers := fixture(t)
	server := onlineServer(t, runtime, servers)

	files := func(operation, name string) model.FileResult {
		t.Helper()

		result, err := runtime.Files(t.Context(), server.SparkID, model.FileRequest{ServerID: server.ID, Operation: operation, Path: name})
		if err != nil {
			t.Fatal(err)
		}

		return result
	}
	if listed := files("backup.list", "."); len(listed.Entries) != 2 {
		t.Fatal("expected seeded backups", listed)
	}

	if _, err := runtime.Action(t.Context(), server.SparkID, model.RuntimeRequest{ServerID: server.ID, Action: "backup", OperationID: "snapshot"}); err == nil {
		t.Fatal("running server backup accepted")
	}

	for _, action := range []string{"stop", "backup", "backup"} {
		if _, err := runtime.Action(t.Context(), server.SparkID, model.RuntimeRequest{ServerID: server.ID, Action: action, OperationID: "snapshot"}); err != nil {
			t.Fatal(err)
		}
	}

	if listed := files("backup.list", "."); len(listed.Entries) != 3 {
		t.Fatal("backup creation was not idempotent", listed)
	}

	download := files("backup.read", "snapshot.tar.gz")

	zipped, err := gzip.NewReader(bytes.NewReader(download.Data))
	if err != nil {
		t.Fatal("download is not gzip", err)
	}

	defer func() { _ = zipped.Close() }()

	archive := tar.NewReader(zipped)
	found := false

	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}

		if err != nil {
			t.Fatal(err)
		}

		if header.Name == "server.properties" {
			contents, err := io.ReadAll(archive)
			if err != nil || !bytes.Contains(contents, []byte("motd=Campfire")) {
				t.Fatal("archive lost game files", err)
			}

			found = true
		}
	}

	if !found || download.Size != int64(len(download.Data)) {
		t.Fatal("incomplete backup download")
	}

	original := files("read", "server.properties")
	if _, err := runtime.Files(t.Context(), server.SparkID, model.FileRequest{ServerID: server.ID, Operation: "write", Path: "server.properties", Revision: original.Revision, Data: []byte("changed")}); err != nil {
		t.Fatal(err)
	}

	runtime, err = New(t.Context(), db.ORM(), auth)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := runtime.Action(t.Context(), server.SparkID, model.RuntimeRequest{ServerID: server.ID, Action: "restore", OperationID: "restore", Content: "snapshot.tar.gz"}); err != nil {
		t.Fatal(err)
	}

	if restored := files("read", "server.properties"); !bytes.Equal(restored.Data, original.Data) {
		t.Fatal("restore did not recover files")
	}

	files("backup.delete", "snapshot.tar.gz")
	files("backup.delete", "snapshot.tar.gz")

	runtime, err = New(t.Context(), db.ORM(), auth)
	if err != nil {
		t.Fatal(err)
	}

	if listed := files("backup.list", "."); len(listed.Entries) != 2 {
		t.Fatal("restart recreated deleted backups", listed)
	}
}
