package runtime

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"campfire/internal/shared/model"
)

func TestSavedataBackupRestoresCapturedRootsAndPreservesInstallation(t *testing.T) {
	m, d := fileTestManager(t)
	m.Ownership = func(string, string) error { return nil }

	if status := awaitOperation(t, m, d, "install", "install"); status.Error != "" {
		t.Fatal(status.Error)
	}

	volume := filepath.Join(m.root(d.ServerID), "data")
	write := func(name, content string) {
		t.Helper()

		if err := os.MkdirAll(filepath.Dir(filepath.Join(volume, name)), 0700); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(filepath.Join(volume, name), []byte(content), 0700); err != nil {
			t.Fatal(err)
		}
	}
	write("saves/test-world/world.dat", "original world")
	write("server.bin", "original binary")

	options := model.BackupOptions{Mode: model.BackupModeSavedata, Paths: []model.SavedataPath{{Path: "saves/test-world", Required: true}, {Path: "optional/config"}}}

	status := awaitBackup(t, m, d, model.RuntimeRequest{Action: model.ServerActionBackup, OperationID: "selected", BackupOptions: options})
	if status.Error != "" {
		t.Fatal(status.Error)
	}

	listed, err := m.Files(t.Context(), d, model.FileRequest{ServerID: d.ServerID, Operation: model.FileOperationBackupList})
	if err != nil || len(listed.Entries) != 1 || listed.Entries[0].BackupMode != model.BackupModeSavedata {
		t.Fatal("save-data mode not retained in listing", listed, err)
	}

	write("saves/test-world/world.dat", "changed world")
	write("saves/test-world/new.chunk", "added after backup")
	write("optional/config/settings.ini", "added optional root")
	write("server.bin", "updated binary")
	write("other-world/world.dat", "another world")

	if err := os.Symlink("server.bin", filepath.Join(volume, "server-link")); err != nil {
		t.Fatal(err)
	}

	status = awaitBackup(t, m, d, model.RuntimeRequest{Action: model.ServerActionRestore, OperationID: "restore-selected", Content: "savedata-selected.tar.gz"})
	if status.Error != "" {
		t.Fatal(status.Error)
	}

	for name, expected := range map[string]string{"saves/test-world/world.dat": "original world", "server.bin": "updated binary", "other-world/world.dat": "another world", "server-link": "updated binary"} {
		data, err := os.ReadFile(filepath.Join(volume, name))
		if err != nil || string(data) != expected {
			t.Fatal("incorrect restore contents", name, string(data), err)
		}
	}

	for _, name := range []string{"saves/test-world/new.chunk", "optional/config", model.BackupMetadataName} {
		if _, err := os.Lstat(filepath.Join(volume, name)); !os.IsNotExist(err) {
			t.Fatal("restore retained data absent from snapshot", name, err)
		}
	}

	info, err := os.Lstat(filepath.Join(volume, "server-link"))
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("installation link was followed or lost", err)
	}
}

func TestSavedataRestoreRejectsInconsistentMetadata(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		metadata string
		entry    string
	}{
		{"outside root", `{"format":1,"mode":"savedata","paths":[{"path":"world","required":true}]}`, "server.bin"},
		{"missing root", `{"format":1,"mode":"savedata","paths":[{"path":"world","required":true}]}`, "world/child"},
		{"missing mode", `{"format":1}`, "world"},
		{"unknown format", `{"format":2,"mode":"full"}`, "world"},
		{"required recorded missing", `{"format":1,"mode":"savedata","paths":[{"path":"world","required":true}],"missing":["world"]}`, "world"},
		{"absent optional has data", `{"format":1,"mode":"savedata","paths":[{"path":"world"}],"missing":["world"]}`, "world/child"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var data bytes.Buffer

			compressed := gzip.NewWriter(&data)
			archive := tar.NewWriter(compressed)

			if err := archive.WriteHeader(&tar.Header{Name: model.BackupMetadataName, Typeflag: tar.TypeReg, Size: int64(len(scenario.metadata)), Mode: 0600}); err != nil {
				t.Fatal(err)
			}

			if _, err := archive.Write([]byte(scenario.metadata)); err != nil {
				t.Fatal(err)
			}

			if err := archive.WriteHeader(&tar.Header{Name: scenario.entry, Typeflag: tar.TypeReg, Mode: 0600}); err != nil {
				t.Fatal(err)
			}

			if err := archive.Close(); err != nil {
				t.Fatal(err)
			}

			if err := compressed.Close(); err != nil {
				t.Fatal(err)
			}

			if err := extractBackup(t.Context(), &data, t.TempDir(), backupBytes); err == nil {
				t.Fatal("accepted an inconsistent save-data snapshot")
			}
		})
	}
}

func TestSavedataBackupRejectsMissingRequiredAndSymlinkPaths(t *testing.T) {
	for _, scenario := range []string{"missing", "symlink"} {
		t.Run(scenario, func(t *testing.T) {
			m, d := fileTestManager(t)
			m.Ownership = func(string, string) error { return nil }

			if status := awaitOperation(t, m, d, "install", "install"); status.Error != "" {
				t.Fatal(status.Error)
			}

			if scenario == "symlink" {
				if err := os.Symlink(t.TempDir(), filepath.Join(m.root(d.ServerID), "data", "saves")); err != nil {
					t.Fatal(err)
				}
			}

			options := model.BackupOptions{Mode: model.BackupModeSavedata, Paths: []model.SavedataPath{{Path: "saves/world", Required: true}}}

			status := awaitBackup(t, m, d, model.RuntimeRequest{Action: model.ServerActionBackup, OperationID: "invalid", BackupOptions: options})
			if status.Error == "" {
				t.Fatal("unsafe or incomplete backup succeeded")
			}

			if _, err := os.Stat(filepath.Join(m.root(d.ServerID), "backups", "savedata-invalid.tar.gz")); !os.IsNotExist(err) {
				t.Fatal("failed backup was published", err)
			}
		})
	}
}
