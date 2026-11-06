package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"campfire/internal/campfire/devfixture"
	"campfire/internal/shared/backuparchive"
	"campfire/internal/shared/model"
)

func backupCapacityFixture(t *testing.T) (*Manager, model.Deployment, []byte) {
	t.Helper()

	m, d := fileTestManager(t)
	ember := model.Ember{ID: d.Configuration.EmberID, Schema: 1, Name: "Test game", Images: map[string]string{"default": d.Configuration.Image}, Startup: d.Ember.Startup, WorkDir: d.Ember.WorkDir, User: d.Ember.User, Variables: []model.Variable{{Name: "SECONDS", Default: "3600", Editable: true}}, Installer: d.Ember.Installer}
	configuration := model.ServerConfiguration{Name: "Test server", EmberID: ember.ID, Image: d.Configuration.Image, Variables: map[string]string{"SECONDS": "3600"}, CPULimit: 1, MemoryMB: 128}
	manifest := backuparchive.Manifest{Format: backuparchive.Format, BackupOptions: model.BackupOptions{Mode: model.BackupModeFull, Definition: model.NewBackupDefinition(model.Server{ServerConfiguration: configuration}, ember)}}

	metadata, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}

	data, err := devfixture.Archive(map[string]string{model.BackupMetadataName: string(metadata)}, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	directory := filepath.Join(m.root(d.ServerID), "backups")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}

	for i := range backupArchiveLimit - 1 {
		if err := os.WriteFile(filepath.Join(directory, fmt.Sprintf("full-fixture-%04d.tar.gz", i)), data, 0600); err != nil {
			t.Fatal(err)
		}
	}

	return m, d, data
}

func requireBackupCapacityError(t *testing.T, err error) {
	t.Helper()

	var failure *model.FileError
	if !errors.As(err, &failure) || failure.Code != model.FileLimitExceeded {
		t.Fatalf("expected backup capacity error, got %v", err)
	}
}

func TestBackupCapacitySharedByCreationAndUploads(t *testing.T) {
	m, d, data := backupCapacityFixture(t)
	call := func(q model.FileRequest) (model.FileResult, error) {
		q.ServerID = d.ServerID
		return m.Files(t.Context(), d, q)
	}
	name := model.BackupModeFull.ArchiveName("last-slot")

	started, err := call(model.FileRequest{Operation: model.FileOperationBackupUploadBegin, Path: name, Size: int64(len(data))})
	if err != nil {
		t.Fatal(err)
	}

	_, err = call(model.FileRequest{Operation: model.FileOperationBackupUploadBegin, Path: model.BackupModeFull.ArchiveName("overflow"), Size: int64(len(data))})
	requireBackupCapacityError(t, err)

	entry, err := m.load(d.ServerID)
	if err != nil {
		t.Fatal(err)
	}

	requireBackupCapacityError(t, m.createBackup(t.Context(), d, "manual", entry, model.BackupOptions{Mode: model.BackupModeFull}))

	listed, err := call(model.FileRequest{Operation: model.FileOperationBackupList})
	if err != nil || len(listed.Entries) != backupArchiveLimit-1 {
		t.Fatalf("pending upload changed completed listing: %d %v", len(listed.Entries), err)
	}

	if _, err := call(model.FileRequest{Operation: model.FileOperationBackupUploadCancel, Path: name, UploadID: started.UploadID}); err != nil {
		t.Fatal(err)
	}

	if err := m.createBackup(t.Context(), d, "manual", entry, model.BackupOptions{Mode: model.BackupModeFull}); err != nil {
		t.Fatal("cancelled upload did not release capacity", err)
	}

	_, err = call(model.FileRequest{Operation: model.FileOperationBackupUploadBegin, Path: name, Size: int64(len(data))})
	requireBackupCapacityError(t, err)

	if _, err := call(model.FileRequest{Operation: model.FileOperationBackupDelete, Path: "full-fixture-0000.tar.gz"}); err != nil {
		t.Fatal(err)
	}

	started, err = call(model.FileRequest{Operation: model.FileOperationBackupUploadBegin, Path: name, Size: int64(len(data))})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := call(model.FileRequest{Operation: model.FileOperationBackupUploadChunk, Path: name, UploadID: started.UploadID, Data: data}); err != nil {
		t.Fatal(err)
	}

	if _, err := call(model.FileRequest{Operation: model.FileOperationBackupUploadCommit, Path: name, UploadID: started.UploadID}); err != nil {
		t.Fatal("reserved upload could not publish at capacity", err)
	}

	listed, err = call(model.FileRequest{Operation: model.FileOperationBackupList})
	if err != nil || len(listed.Entries) != backupArchiveLimit {
		t.Fatalf("full backup catalog became unreadable: %d %v", len(listed.Entries), err)
	}
}

func TestBackupUploadRechecksCapacityBeforePublication(t *testing.T) {
	m, d, data := backupCapacityFixture(t)
	call := func(q model.FileRequest) (model.FileResult, error) {
		q.ServerID = d.ServerID
		return m.Files(t.Context(), d, q)
	}
	name := model.BackupModeFull.ArchiveName("last-slot")

	started, err := call(model.FileRequest{Operation: model.FileOperationBackupUploadBegin, Path: name, Size: int64(len(data))})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := call(model.FileRequest{Operation: model.FileOperationBackupUploadChunk, Path: name, UploadID: started.UploadID, Data: data}); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(m.root(d.ServerID), "backups", "full-external.tar.gz"), data, 0600); err != nil {
		t.Fatal(err)
	}

	_, err = call(model.FileRequest{Operation: model.FileOperationBackupUploadCommit, Path: name, UploadID: started.UploadID})
	requireBackupCapacityError(t, err)

	if _, err := os.Stat(filepath.Join(m.root(d.ServerID), "backups", name)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("overflow archive was published", err)
	}

	listed, err := call(model.FileRequest{Operation: model.FileOperationBackupList})
	if err != nil || len(listed.Entries) != backupArchiveLimit {
		t.Fatalf("staging file made the full catalog unreadable: %d %v", len(listed.Entries), err)
	}
}
