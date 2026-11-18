package fileops

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"campfire/internal/shared/model"
)

func TestDirectExtractionLeavesAnIncompleteCurrentFileOnDecodeFailure(t *testing.T) {
	directory := t.TempDir()

	var buffer bytes.Buffer

	compressed := gzip.NewWriter(&buffer)

	archive := tar.NewWriter(compressed)
	if err := archive.WriteHeader(&tar.Header{Name: "partial.txt", Size: 10, Mode: 0600}); err != nil {
		t.Fatal(err)
	}

	if _, err := archive.Write([]byte("abc")); err != nil {
		t.Fatal(err)
	}

	if err := archive.Close(); err == nil {
		t.Fatal("fixture should be incomplete")
	}

	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(directory, "input.tgz"), buffer.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}

	m := New(Host{FileOwner: func(*os.File, string) error { return nil }})
	defer m.Close()

	q := model.FileRequest{ServerID: "server", Operation: model.FileOperationExtractStart, Path: "input.tgz", Destination: ".", ExtractionOwner: "owner", Extraction: &model.ExtractionRequest{ID: "12345678901234567890123456789012"}}
	if _, err := m.Run(t.Context(), model.Deployment{}, directory, q); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		q.Operation = model.FileOperationExtractStatus

		result, err := m.Run(t.Context(), model.Deployment{}, directory, q)
		if err != nil {
			t.Fatal(err)
		}

		if result.Extraction.State == "failed" {
			data, err := os.ReadFile(filepath.Join(directory, "partial.txt"))
			if err != nil || string(data) != "abc" {
				t.Fatal("partial file was removed or misreported", string(data), err)
			}

			if len(result.Extraction.Outcomes) != 1 || result.Extraction.Outcomes[0].Status != "failed" {
				t.Fatal(result.Extraction)
			}

			return
		}

		time.Sleep(time.Millisecond)
	}

	t.Fatal("incomplete archive did not fail")
}
