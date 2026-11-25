package runtime

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"campfire/internal/shared/model"
)

func sparseTestManager(t *testing.T, d model.Deployment) *Manager {
	t.Helper()

	if root := os.Getenv("CAMPFIRE_TEST_STORAGE_ROOT"); root != "" {
		base, err := os.MkdirTemp(root, "spark-roundtrip-")
		if err != nil {
			t.Fatal(err)
		}

		m := New(t.Context(), base)
		quota := m.Quota

		t.Cleanup(func() {
			m.Close()

			if err := removeQuotaData(context.Background(), quota, filepath.Join(m.root(d.ServerID), "data")); err != nil {
				t.Error(err)
			}

			if err := os.RemoveAll(base); err != nil {
				t.Error(err)
			}
		})

		if !m.DiskQuotaSupported() {
			t.Fatal("expected a quota-enabled filesystem", m.quotaUnavailable)
		}

		if err := os.MkdirAll(m.root(d.ServerID), 0700); err != nil {
			t.Fatal(err)
		}

		m.Docker = &fakeDocker{states: make(map[string]string)}
		m.FileOwner = func(*os.File, string) error { return nil }

		return m
	}

	m, _ := fileTestManager(t)
	m.Quota = &accountingQuota{DiskQuota: fakeQuota{}, bytes: 256 << 10}

	return m
}

// The save has a leading hole, data crossing a copy-buffer boundary, and a
// trailing hole. Its apparent size exceeds the allocation on both Sparks.
func TestSparseSaveBackupTransferAndFileWrites(t *testing.T) {
	d := runtimeFixture()
	m := sparseTestManager(t, d)
	m.Ownership = func(string, string) error { return nil }

	d.Configuration.DiskMB = 8
	if status := awaitOperation(t, m, d, "install", "install"); status.Error != "" {
		t.Fatal(status.Error)
	}

	data := filepath.Join(m.root(d.ServerID), "data")
	world := filepath.Join(data, "world")
	want := make([]byte, 16<<20+17)
	payload := bytes.Repeat([]byte("game-save"), 1024)

	const offset = (64 << 10) - 3
	copy(want[offset:], payload)

	file, err := os.Create(world)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := file.WriteAt(payload, offset); err != nil {
		t.Fatal(err)
	}

	if err := file.Truncate(int64(len(want))); err != nil {
		t.Fatal(err)
	}

	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	checkWorld := func(name string) {
		t.Helper()

		var stat syscall.Stat_t
		if err := syscall.Stat(name, &stat); err != nil {
			t.Fatal(err)
		}

		if allocated := stat.Blocks * 512; allocated >= d.Configuration.DiskMB<<20 {
			t.Fatalf("restored save occupies %d bytes under an %d MiB allocation", allocated, d.Configuration.DiskMB)
		}

		got, err := os.ReadFile(name)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatal("restored save differs", err)
		}
	}

	call := func(q model.FileRequest) model.FileResult {
		t.Helper()

		q.ServerID = d.ServerID

		r, err := m.Files(t.Context(), d, q)
		if err != nil {
			t.Fatal(err)
		}

		return r
	}
	call(model.FileRequest{Operation: model.FileOperationWrite, Path: "edited", Data: []byte("a"), Revision: "absent"})
	upload := call(model.FileRequest{Operation: model.FileOperationUploadBegin, Path: "uploaded", Revision: "absent", Size: 1})
	call(model.FileRequest{Operation: model.FileOperationUploadChunk, Path: "uploaded", UploadID: upload.UploadID, Data: []byte("b")})
	call(model.FileRequest{Operation: model.FileOperationUploadCommit, Path: "uploaded", UploadID: upload.UploadID})
	call(model.FileRequest{Operation: model.FileOperationCompress, Path: "edited", Destination: "edit.zip"})
	call(model.FileRequest{Operation: model.FileOperationExtract, Path: "edit.zip", Destination: "unpacked"})

	if status := awaitOperation(t, m, d, "backup", "snapshot"); status.Error != "" {
		t.Fatal(status.Error)
	}

	if err := os.Remove(world); err != nil {
		t.Fatal(err)
	}

	if status := awaitBackup(t, m, d, model.RuntimeRequest{Action: "restore", OperationID: "restore", Content: "full-snapshot.tar.gz"}); status.Error != "" {
		t.Fatal(status.Error)
	}

	checkWorld(world)

	selected := model.BackupOptions{Mode: model.BackupModeSavedata, Paths: []model.SavedataPath{{Path: "edited", Required: true}}}
	if status := awaitBackup(t, m, d, model.RuntimeRequest{Action: "backup", OperationID: "selected", BackupOptions: selected}); status.OperationState != model.OperationSucceeded {
		t.Fatal("save-data backup failed", status)
	}

	if status := awaitBackup(t, m, d, model.RuntimeRequest{Action: "restore", OperationID: "restore-selected", Content: selected.Mode.ArchiveName("selected")}); status.OperationState != model.OperationSucceeded {
		t.Fatal("save-data restore could not preserve the sparse installation file", status)
	}

	checkWorld(world)

	job := &localTransfer{JobResult: model.JobResult{ID: "transfer", ServerID: d.ServerID}, Installed: true}
	if err := m.exportTransfer(t.Context(), d, job); err != nil {
		t.Fatal(err)
	}

	target := sparseTestManager(t, d)
	nativeQuota := target.Quota
	target.Ownership = m.Ownership

	archive, err := os.ReadFile(m.transferArchive(d.ServerID, job.ID))
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(target.transferArchive(d.ServerID, job.ID), archive, 0600); err != nil {
		t.Fatal(err)
	}
	// The fallback has no native accounting. Its logical limit
	// must continue to protect a destination importing the very same archive.
	target.Quota = nil
	if err := target.importTransfer(t.Context(), d, job); err == nil {
		t.Fatal("unmanaged destination accepted an over-allocation save")
	}

	target.Quota = nativeQuota
	if err := target.importTransfer(t.Context(), d, job); err != nil {
		t.Fatal(err)
	}

	targetData := filepath.Join(target.root(d.ServerID), "data")
	checkWorld(filepath.Join(targetData, "world"))

	for name, value := range map[string]string{"edited": "a", "uploaded": "b"} {
		got, err := os.ReadFile(filepath.Join(targetData, name))
		if err != nil || string(got) != value {
			t.Fatalf("transferred %s: %q, %v", name, got, err)
		}
	}

	target.Quota = fakeQuota{}
	_, err = target.Files(t.Context(), d, model.FileRequest{ServerID: d.ServerID, Operation: model.FileOperationWrite, Path: "new", Revision: "absent", Data: []byte("c")})

	var fileErr *model.FileError
	if !errors.As(err, &fileErr) || fileErr.Code != model.FileLimitExceeded {
		t.Fatalf("unmanaged write should exceed the logical allocation: %v", err)
	}
}
