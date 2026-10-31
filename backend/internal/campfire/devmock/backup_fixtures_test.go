//go:build !production

package devmock

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

func TestMockBackupExamplesHaveAppliedPathsAndBothArchiveModes(t *testing.T) {
	runtime, db, auth, servers := fixture(t)

	var server model.Server

	for _, item := range servers {
		if item.ID == "creative" {
			server = item
		}
	}

	if server.ID == "" || server.EmberID != "mock-current" || server.EmberUpdateAvailable {
		t.Fatal("missing an applied backup example server", server)
	}

	ember, err := repository.NewEmberRepository(db.ORM()).Get(t.Context(), server.EmberID)
	if err != nil || len(ember.SavedataPaths) == 0 {
		t.Fatal("backup example has no declared save-data paths", ember, err)
	}

	listed, err := runtime.Files(t.Context(), server.SparkID, model.FileRequest{ServerID: server.ID, Operation: model.FileOperationBackupList, Path: "."})
	if err != nil || len(listed.Entries) != 2 {
		t.Fatal("missing backup examples", listed, err)
	}

	modes := make(map[model.BackupMode]bool)

	for _, entry := range listed.Entries {
		if !strings.HasPrefix(entry.Name, string(entry.BackupMode)+"-") {
			t.Fatal("example is missing its mode prefix", entry)
		}

		modes[entry.BackupMode] = true

		download, err := runtime.Files(t.Context(), server.SparkID, model.FileRequest{ServerID: server.ID, Operation: model.FileOperationBackupRead, Path: entry.Name})
		if err != nil {
			t.Fatal(err)
		}

		compressed, err := gzip.NewReader(bytes.NewReader(download.Data))
		if err != nil {
			t.Fatal(err)
		}

		archive := tar.NewReader(compressed)

		header, err := archive.Next()
		if err != nil || header.Name != model.BackupMetadataName {
			t.Fatal("example is missing its archive metadata", header, err)
		}

		var metadata struct {
			model.BackupOptions
		}
		if err := json.NewDecoder(archive).Decode(&metadata); err != nil || metadata.Mode != entry.BackupMode {
			t.Fatal("archive and catalog modes differ", metadata, err)
		}

		files := make(map[string]bool)

		for {
			header, err := archive.Next()
			if errors.Is(err, io.EOF) {
				break
			}

			if err != nil {
				t.Fatal(err)
			}

			files[header.Name] = true
		}

		if err := compressed.Close(); err != nil {
			t.Fatal(err)
		}

		if !files["world/level.dat"] || files["server.jar"] != (entry.BackupMode == model.BackupModeFull) {
			t.Fatal("example archive does not match its declared mode", entry.BackupMode, files)
		}
	}

	if !modes[model.BackupModeFull] || !modes[model.BackupModeSavedata] {
		t.Fatal("both backup modes must be available", modes)
	}

	if _, err := runtime.Files(t.Context(), server.SparkID, model.FileRequest{ServerID: server.ID, Operation: model.FileOperationBackupDelete, Path: listed.Entries[0].Name}); err != nil {
		t.Fatal(err)
	}

	runtime, err = New(t.Context(), db.ORM(), auth)
	if err != nil {
		t.Fatal(err)
	}

	listed, err = runtime.Files(t.Context(), server.SparkID, model.FileRequest{ServerID: server.ID, Operation: model.FileOperationBackupList, Path: "."})
	if err != nil || len(listed.Entries) != 1 {
		t.Fatal("restarting mock mode recreated a deleted example", listed, err)
	}
}
