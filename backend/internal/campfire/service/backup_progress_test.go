package service

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"campfire/internal/shared/model"
	"campfire/internal/shared/workprogress"
)

type pendingBackupAcknowledgement struct {
	*memoryBackupStorage
	entered chan struct{}
	release chan struct{}
}

func (s *pendingBackupAcknowledgement) Put(ctx context.Context, key string, size int64, digest string, reader io.Reader) (string, error) {
	if _, err := s.memoryBackupStorage.Put(ctx, key, size, digest, reader); err != nil {
		return "", err
	}

	close(s.entered)

	select {
	case <-s.release:
	case <-ctx.Done():
		return "", ctx.Err()
	}

	return "", errors.New("lost publication acknowledgement")
}

func TestBackupStreamingProgressIsNotPublicationAcknowledgement(t *testing.T) {
	f, service, remote, _ := backupFixture(t)
	seedBackup(t, f, "snapshot", time.Now())

	if err := service.SavePolicy(t.Context(), f.server.ID, model.BackupPolicy{Storage: model.BackupStorageS3}); err != nil {
		t.Fatal(err)
	}

	storage := &pendingBackupAcknowledgement{memoryBackupStorage: remote, entered: make(chan struct{}), release: make(chan struct{})}
	service.Remote = storage

	var once sync.Once

	unblock := func() { once.Do(func() { close(storage.release) }) }
	defer unblock()

	done := make(chan error, 1)
	go func() { done <- service.reconcile(t.Context(), f.server.ID) }()

	select {
	case <-storage.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("upload never consumed input")
	}

	rows, err := service.Catalog(t.Context())
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}

	progress := rows[0].WorkProgress
	if rows[0].Remote || rows[0].State != model.BackupSyncing || progress == nil || progress.Phase != workprogress.Uploading || progress.InputBytes != rows[0].Size || progress.InputTotal != rows[0].Size {
		t.Fatal(rows[0], progress)
	}

	unblock()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reconcile did not finish")
	}

	rows, err = service.Catalog(t.Context())
	if err != nil || len(rows) != 1 || rows[0].Remote || !rows[0].Local || rows[0].WorkProgress != nil || rows[0].State != model.BackupError {
		t.Fatal(rows, err)
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	if len(service.progress) != 0 {
		t.Fatal("terminal progress leaked", service.progress)
	}
}
