package runtime

import (
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"campfire/internal/shared/model"
)

func TestExtractionExcludesFilesystemLifecycleAndTransfers(t *testing.T) {
	f := newExtractionFixture(t, map[string]string{"a.txt": "new"})
	f.write(t, "existing/a.txt", "old")
	f.call(t, model.FileOperationExtractStart, nil)
	f.wait(t, "waiting")

	for _, action := range []model.ServerAction{model.ServerActionDelete, model.ServerActionRestore, model.ServerActionInstall, model.ServerActionReinstall, model.ServerActionUpdate, model.ServerActionBackup} {
		t.Run(string(action), func(t *testing.T) {
			_, err := f.m.Execute(t.Context(), f.d, model.RuntimeRequest{ServerID: f.d.ServerID, OperationID: "blocked-" + string(action), Action: action})
			if err == nil || !strings.Contains(err.Error(), "extraction") {
				t.Fatal("filesystem lifecycle operation overlapped extraction")
			}
		})
	}

	for _, operation := range []model.JobOperation{model.JobOperationExport, model.JobOperationPrepare} {
		_, err := f.m.Job(t.Context(), f.d, model.JobRequest{ID: "blocked-transfer", ServerID: f.d.ServerID, Operation: operation})
		if err == nil {
			t.Fatal("transfer overlapped extraction", operation)
		}
	}

	// Observation and normal file edits remain available, including stale-conflict checks.
	if _, err := f.m.Execute(t.Context(), f.d, model.RuntimeRequest{ServerID: f.d.ServerID, Action: model.ServerActionStatus}); err != nil {
		t.Fatal(err)
	}

	if _, err := f.m.Files(t.Context(), f.d, model.FileRequest{ServerID: f.d.ServerID, Operation: model.FileOperationList, Path: "existing"}); err != nil {
		t.Fatal(err)
	}

	f.call(t, model.FileOperationExtractCancel, nil)
	f.read(t, "existing/a.txt", "old")
}

func TestCancelledExtractionKeepsWriterLeaseUntilDrained(t *testing.T) {
	f := newExtractionFixture(t, map[string]string{"a.txt": "new"})
	entered, release := make(chan struct{}), make(chan struct{})

	var once sync.Once

	f.m.FileOwner = func(file *os.File, _ string) error {
		info, err := file.Stat()
		if err == nil && info.Mode().IsRegular() {
			once.Do(func() { close(entered); <-release })
		}

		return err
	}

	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()

	f.call(t, model.FileOperationExtractStart, nil)

	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("writer never started")
	}

	status := f.call(t, model.FileOperationExtractCancel, nil)
	if status.State != "cancelled" || !f.m.fileOperations.Active(f.d.ServerID) {
		t.Fatal("cancellation released a live writer")
	}

	q := f.q

	q.Extraction = &model.ExtractionRequest{ID: "another-operation-12345678901234567890"}
	if _, err := f.m.Files(t.Context(), f.d, q); err == nil {
		t.Fatal("second extraction started before cancelled writer drained")
	}

	if err := f.m.CanUpdate(); err == nil {
		t.Fatal("self-update allowed with a live writer")
	}

	if _, err := f.m.Execute(t.Context(), f.d, model.RuntimeRequest{ServerID: f.d.ServerID, OperationID: "blocked-delete", Action: model.ServerActionDelete}); err == nil {
		t.Fatal("deletion allowed with a live writer")
	}

	close(release)

	deadline := time.Now().Add(time.Second)
	for f.m.fileOperations.Active(f.d.ServerID) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	if f.m.fileOperations.Active(f.d.ServerID) {
		t.Fatal("drained writer retained its lease")
	}

	if err := f.m.CanUpdate(); err != nil {
		t.Fatal(err)
	}

	if _, err := f.m.Files(t.Context(), f.d, q); err != nil {
		t.Fatal("drained writer blocked subsequent extraction", err)
	}
}
