package fileops

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"campfire/internal/shared/model"
)

func TestCompressionProgressPublicationAndOwnerBinding(t *testing.T) {
	for _, name := range []string{"archive.tar.gz", "archive.zip"} {
		t.Run(name, func(t *testing.T) { testCompressionProgressPublicationAndOwnerBinding(t, name) })
	}
}

func testCompressionProgressPublicationAndOwnerBinding(t *testing.T, archiveName string) {
	volume := t.TempDir()
	if err := os.WriteFile(filepath.Join(volume, "source.txt"), []byte("payload"), 0600); err != nil {
		t.Fatal(err)
	}

	reached, release := make(chan struct{}), make(chan struct{})

	m := New(Host{FileOwner: func(*os.File, string) error { close(reached); <-release; return nil }})
	defer m.Close()
	defer close(release)

	d := model.Deployment{ServerID: "server"}
	q := model.FileRequest{ServerID: d.ServerID, Operation: model.FileOperationCompressStart, Path: "source.txt", Destination: archiveName, CompressionOwner: "owner", Compression: &model.CompressionRequest{ID: "12345678901234567890123456789012"}}

	result, err := m.Files(t.Context(), d, volume, &State{}, q)
	if err != nil || result.Compression.State != "running" {
		t.Fatal(result, err)
	}

	select {
	case <-reached:
	case <-time.After(time.Second):
		t.Fatal("writer not reached")
	}

	q.Operation = model.FileOperationCompressStatus

	result, err = m.Files(t.Context(), d, volume, &State{}, q)
	if err != nil || result.Compression.WorkProgress == nil || result.Compression.WorkProgress.InputBytes != 7 || result.Compression.WorkProgress.OutputBytes == 0 {
		t.Fatal(result, err)
	}

	if !m.Active(d.ServerID) {
		t.Fatal("lost writer lease")
	}

	if _, err = os.Stat(filepath.Join(volume, q.Destination)); !os.IsNotExist(err) {
		t.Fatal("archive published before confirmation", err)
	}

	unauthorized := q

	unauthorized.CompressionOwner = "other"
	if _, err = m.Files(t.Context(), d, volume, &State{}, unauthorized); err == nil {
		t.Fatal("foreign owner observed progress")
	}

	q.Operation = model.FileOperationCompressCancel

	result, err = m.Files(t.Context(), d, volume, &State{}, q)
	if err != nil || result.Compression.State != "cancelling" || !m.Active(d.ServerID) {
		t.Fatal("cancel released live writer", result, err)
	}

	release <- struct{}{}

	m.wg.Wait()

	if m.Active(d.ServerID) {
		t.Fatal("writer did not drain")
	}

	if _, err = os.Stat(filepath.Join(volume, q.Destination)); !os.IsNotExist(err) {
		t.Fatal("cancelled archive published", err)
	}

	entries, err := os.ReadDir(volume)
	if err != nil || len(entries) != 1 {
		t.Fatal("staging was not cleaned", entries, err)
	}
}

func TestCompressionCompletionIsIdempotentAndCollisionSafe(t *testing.T) {
	for _, name := range []string{"archive.tar.gz", "archive.zip"} {
		t.Run(name, func(t *testing.T) { testCompressionCompletionIsIdempotentAndCollisionSafe(t, name) })
	}
}

func testCompressionCompletionIsIdempotentAndCollisionSafe(t *testing.T, archiveName string) {
	for _, collision := range []bool{false, true} {
		t.Run(map[bool]string{false: "publish", true: "collision"}[collision], func(t *testing.T) {
			volume := t.TempDir()
			if err := os.WriteFile(filepath.Join(volume, "source"), []byte("payload"), 0600); err != nil {
				t.Fatal(err)
			}

			target := filepath.Join(volume, archiveName)
			if collision {
				if err := os.WriteFile(target, []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
			}

			m := New(Host{FileOwner: func(*os.File, string) error { return nil }})
			defer m.Close()

			d := model.Deployment{ServerID: "server"}

			q := model.FileRequest{ServerID: d.ServerID, Operation: model.FileOperationCompressStart, Path: "source", Destination: archiveName, CompressionOwner: "owner", Compression: &model.CompressionRequest{ID: "12345678901234567890123456789012"}}
			if _, err := m.Files(t.Context(), d, volume, &State{}, q); err != nil {
				t.Fatal(err)
			}

			q.Operation = model.FileOperationCompressStatus
			deadline := time.Now().Add(time.Second)

			var result model.FileResult

			for {
				var err error

				result, err = m.Files(t.Context(), d, volume, &State{}, q)
				if err != nil {
					t.Fatal(err)
				}

				if result.Compression.State != "running" {
					break
				}

				if time.Now().After(deadline) {
					t.Fatal("operation did not finish")
				}

				time.Sleep(time.Millisecond)
			}

			expected := model.CompressionCompleted
			if collision {
				expected = model.CompressionFailed
			}

			if result.Compression.State != expected || m.Active(d.ServerID) {
				t.Fatal(result)
			}

			q.Operation = model.FileOperationCompressStart

			replay, err := m.Files(context.Background(), d, volume, &State{}, q)
			if err != nil || replay.Compression.State != expected {
				t.Fatal("start was not idempotent", replay, err)
			}

			if collision {
				data, _ := os.ReadFile(target)
				if string(data) != "keep" {
					t.Fatal("destination overwritten")
				}
			}
		})
	}
}
