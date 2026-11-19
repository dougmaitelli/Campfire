package fileops

import (
	"errors"
	"testing"
	"time"

	"campfire/internal/shared/limits"
	"campfire/internal/shared/model"
)

func TestCompressionResultRetentionDoesNotRenewOnPoll(t *testing.T) {
	m := New(Host{})
	defer m.Close()

	d := model.Deployment{ServerID: "server"}
	q := model.FileRequest{ServerID: d.ServerID, Operation: model.FileOperationCompressStatus, Path: "source", Destination: "archive.tar.gz", CompressionOwner: "owner", Compression: &model.CompressionRequest{ID: "12345678901234567890123456789012"}}
	finished := time.Now().Add(-limits.ArchiveResultRetention / 2)
	s := &compressionSession{request: q, finished: finished, status: model.CompressionStatus{ID: q.Compression.ID, State: model.CompressionCompleted}}
	m.compressions[q.Compression.ID] = s

	for range 2 {
		result, err := m.Files(t.Context(), d, "", &State{}, q)
		if err != nil || result.Compression.State != model.CompressionCompleted {
			t.Fatal("retained result unavailable", result, err)
		}

		s.mu.Lock()
		unchanged := s.finished.Equal(finished)
		s.mu.Unlock()

		if !unchanged {
			t.Fatal("polling renewed result retention")
		}
	}

	s.mu.Lock()
	s.finished = time.Now().Add(-limits.ArchiveResultRetention - time.Second)
	s.mu.Unlock()

	var failure *model.FileError
	if _, err := m.Files(t.Context(), d, "", &State{}, q); !errors.As(err, &failure) || failure.Code != model.FileNotFound {
		t.Fatal("expired result remained available", err)
	}

	if len(m.compressions) != 0 {
		t.Fatal("expired result still occupies admission capacity")
	}
}
