package runtime

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"campfire/internal/campfire/devfixture"
	"campfire/internal/shared/backuparchive"
	"campfire/internal/shared/model"
)

func TestImportedPortableBackupRestoresUninstalledServer(t *testing.T) {
	for _, mode := range []model.BackupMode{model.BackupModeFull, model.BackupModeSavedata} {
		t.Run(string(mode), func(t *testing.T) {
			m, d := fileTestManager(t)
			m.Ownership = func(string, string) error { return nil }
			ember := model.Ember{ID: d.Configuration.EmberID, Schema: 1, Name: "Test game", Images: map[string]string{"default": d.Configuration.Image}, Startup: d.Ember.Startup, WorkDir: d.Ember.WorkDir, User: d.Ember.User, Variables: []model.Variable{{Name: "SECONDS", Default: "3600", Editable: true}}, Installer: d.Ember.Installer}
			configuration := model.ServerConfiguration{Name: "Test server", EmberID: ember.ID, Image: d.Configuration.Image, Variables: map[string]string{"SECONDS": "3600"}, CPULimit: 1, MemoryMB: 128}

			options := model.BackupOptions{Mode: mode, Definition: model.NewBackupDefinition(model.Server{ServerConfiguration: configuration}, ember)}
			if mode == model.BackupModeSavedata {
				options.Paths = []model.SavedataPath{{Path: "world", Required: true}}
			}

			manifest := backuparchive.Manifest{Format: backuparchive.Format, BackupOptions: options}

			metadata, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}

			data, err := devfixture.Archive(map[string]string{model.BackupMetadataName: string(metadata), "world/": "", "world/save.dat": "saved world"}, time.Now())
			if err != nil {
				t.Fatal(err)
			}

			name := mode.ArchiveName("imported")
			call := func(q model.FileRequest) model.FileResult {
				t.Helper()

				q.ServerID = d.ServerID

				result, err := m.Files(t.Context(), d, q)
				if err != nil {
					t.Fatal(err)
				}

				return result
			}
			begun := call(model.FileRequest{Operation: model.FileOperationBackupUploadBegin, Path: name, Size: int64(len(data))})
			call(model.FileRequest{Operation: model.FileOperationBackupUploadChunk, Path: name, UploadID: begun.UploadID, Data: data})
			call(model.FileRequest{Operation: model.FileOperationBackupUploadCommit, Path: name, UploadID: begun.UploadID})

			listed := call(model.FileRequest{Operation: model.FileOperationBackupList})
			if len(listed.Entries) != 1 || !listed.Entries[0].Portable {
				t.Fatal("import missing from portable catalog", listed)
			}

			status := awaitBackup(t, m, d, model.RuntimeRequest{Action: model.ServerActionRestore, OperationID: "restore", Content: name})
			if status.Error != "" || status.State != model.RuntimeStateStopped {
				t.Fatal("fresh restore failed", status)
			}

			saved, err := os.ReadFile(filepath.Join(m.root(d.ServerID), "data/world/save.dat"))
			if err != nil || !bytes.Equal(saved, []byte("saved world")) {
				t.Fatal("world not restored", err)
			}

			docker, ok := m.Docker.(*fakeDocker)
			if !ok {
				t.Fatal("expected fake Docker transport")
			}

			docker.mu.Lock()

			installed := false
			for _, args := range docker.calls {
				installed = installed || strings.Contains(strings.Join(args, " "), "-install")
			}
			docker.mu.Unlock()

			if installed != (mode == model.BackupModeSavedata) {
				t.Fatal("wrong installer behavior", mode, installed)
			}

			e, err := m.load(d.ServerID)
			if err != nil {
				t.Fatal(err)
			}

			e.mu.Lock()
			installed = e.Installed
			e.mu.Unlock()

			if !installed {
				t.Fatal("restored server not marked installed")
			}

			if _, err := os.Stat(filepath.Join(m.root(d.ServerID), "data", model.BackupMetadataName)); !os.IsNotExist(err) {
				t.Fatal("portable secrets were extracted into game data", err)
			}
		})
	}
}
