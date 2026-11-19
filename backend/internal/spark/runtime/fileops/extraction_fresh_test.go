package fileops

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"campfire/internal/shared/model"
)

func TestFreshExtractionPreservesLargeMemberLimitAndBoundedResults(t *testing.T) {
	volume := t.TempDir()

	file, err := os.Create(filepath.Join(volume, "archive.zip"))
	if err != nil {
		t.Fatal(err)
	}

	archive := zip.NewWriter(file)
	for i := 0; i < extractionLimit+1; i++ {
		if _, err = archive.Create(fmt.Sprintf("file-%d", i)); err != nil {
			t.Fatal(err)
		}
	}

	if err = archive.Close(); err != nil {
		t.Fatal(err)
	}

	if err = file.Close(); err != nil {
		t.Fatal(err)
	}

	m := New(Host{FileOwner: func(*os.File, string) error { return nil }})
	defer m.Close()

	d := model.Deployment{ServerID: "server"}

	q := model.FileRequest{ServerID: d.ServerID, Operation: model.FileOperationExtractStart, Path: "archive.zip", Destination: "fresh", ExtractionOwner: "owner", Extraction: &model.ExtractionRequest{ID: "12345678901234567890123456789012", Fresh: true}}
	if _, err = m.Files(t.Context(), d, volume, &State{}, q); err != nil {
		t.Fatal(err)
	}

	q.Operation = model.FileOperationExtractStatus
	deadline := time.Now().Add(10 * time.Second)

	for {
		result, err := m.Files(t.Context(), d, volume, &State{}, q)
		if err != nil {
			t.Fatal(err)
		}

		status := result.Extraction
		if status.State.Terminal() {
			if status.State != model.ExtractionCompleted || len(status.Outcomes) != 1 || status.RemainingEntries != 0 || status.WorkProgress == nil || status.WorkProgress.Entries != extractionLimit+1 {
				t.Fatalf("unexpected result: %+v", status)
			}

			break
		}

		if time.Now().After(deadline) {
			t.Fatal("fresh extraction timed out")
		}

		time.Sleep(time.Millisecond)
	}

	entries, err := os.ReadDir(filepath.Join(volume, "fresh"))
	if err != nil || len(entries) != extractionLimit+1 {
		t.Fatal(len(entries), err)
	}
	// A new request cannot merge into a destination created by an earlier run.
	q.Operation = model.FileOperationExtractStart

	q.Extraction = &model.ExtractionRequest{ID: "22345678901234567890123456789012", Fresh: true}
	if _, err = m.Files(t.Context(), d, volume, &State{}, q); err != nil {
		t.Fatal(err)
	}

	m.wg.Wait()

	q.Operation = model.FileOperationExtractStatus

	result, err := m.Files(t.Context(), d, volume, &State{}, q)
	if err != nil || result.Extraction.State != model.ExtractionFailed {
		t.Fatal(result, err)
	}
}
