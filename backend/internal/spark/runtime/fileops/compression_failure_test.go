package fileops

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"campfire/internal/shared/model"
)

func TestCompressionDistinguishesDeadlineFromCancellation(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		name := "cancelled"
		if deadline {
			name = "deadline"
		}

		t.Run(name, func(t *testing.T) {
			volume := t.TempDir()
			if err := os.WriteFile(filepath.Join(volume, "source.txt"), []byte("data"), 0600); err != nil {
				t.Fatal(err)
			}

			m := New(Host{FileOwner: func(*os.File, string) error { return nil }})
			defer m.Close()

			ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
			if !deadline {
				cancel()
				ctx, cancel = context.WithCancel(t.Context())
				cancel()
			}
			defer cancel()

			s := &compressionSession{request: model.FileRequest{ServerID: "server", Path: "source.txt", Destination: "archive.tar.gz"}, status: model.CompressionStatus{ID: "operation", State: model.CompressionRunning}, cancel: cancel}
			m.compress(ctx, model.Deployment{ServerID: "server"}, volume, s)

			expected := model.CompressionCancelled
			if deadline {
				expected = model.CompressionFailed

				if !strings.Contains(s.status.Message, "time limit") {
					t.Fatal("deadline failure has no actionable explanation", s.status)
				}
			}

			if s.status.State != expected || !s.status.State.Terminal() || s.finished.IsZero() || s.status.WorkProgress != nil {
				t.Fatalf("unexpected outcome: %+v", s.status)
			}

			if _, err := os.Stat(filepath.Join(volume, "archive.tar.gz")); !os.IsNotExist(err) {
				t.Fatal("stopped operation published an archive", err)
			}
		})
	}
}
