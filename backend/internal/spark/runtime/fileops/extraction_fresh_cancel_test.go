package fileops

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"campfire/internal/shared/model"
)

func TestFreshCancellationWaitsForWriterAndKeepsPartialDirectory(t *testing.T) {
	volume := t.TempDir()
	if err := os.WriteFile(filepath.Join(volume, "source.zip"), []byte("not decoded before cancellation"), 0600); err != nil {
		t.Fatal(err)
	}

	reached, release := make(chan struct{}), make(chan struct{})

	m := New(Host{FileOwner: func(*os.File, string) error { close(reached); <-release; return nil }})
	defer m.Close()
	defer close(release)

	d := model.Deployment{ServerID: "server"}

	q := model.FileRequest{ServerID: d.ServerID, Operation: model.FileOperationExtractStart, Path: "source.zip", Destination: "fresh", ExtractionOwner: "owner", Extraction: &model.ExtractionRequest{ID: "12345678901234567890123456789012", Fresh: true}}
	if _, err := m.Files(t.Context(), d, volume, &State{}, q); err != nil {
		t.Fatal(err)
	}

	select {
	case <-reached:
	case <-time.After(time.Second):
		t.Fatal("writer not reached")
	}

	q.Operation = model.FileOperationExtractCancel

	result, err := m.Files(t.Context(), d, volume, &State{}, q)
	if err != nil || result.Extraction.State != model.ExtractionCancelled || result.Extraction.RemainingEntries != 1 || !m.Active(d.ServerID) {
		t.Fatal("cancellation released a live writer", result, err)
	}

	release <- struct{}{}

	m.wg.Wait()

	q.Operation = model.FileOperationExtractStatus

	result, err = m.Files(t.Context(), d, volume, &State{}, q)
	if err != nil || result.Extraction.State != model.ExtractionCancelled || result.Extraction.RemainingEntries != 0 || m.Active(d.ServerID) {
		t.Fatal("writer did not drain", result, err)
	}

	if info, err := os.Stat(filepath.Join(volume, "fresh")); err != nil || !info.IsDir() {
		t.Fatal("partial directory was unexpectedly rolled back", err)
	}
}
