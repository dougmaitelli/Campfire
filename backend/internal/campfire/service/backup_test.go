package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

type memoryBackupStorage struct {
	mu                        sync.Mutex
	objects                   map[string][]byte
	fail, failDelete, lostAck bool
	writes                    int
}

func (s *memoryBackupStorage) ID() string { return "test-store" }
func (s *memoryBackupStorage) Put(_ context.Context, key string, size int64, digest string, input io.Reader) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.fail {
		return "", errors.New("storage unavailable")
	}

	if _, exists := s.objects[key]; exists {
		return "version-1", nil
	}

	data, err := io.ReadAll(input)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(data)
	if int64(len(data)) != size || hex.EncodeToString(hash[:]) != digest {
		return "", errors.New("checksum mismatch")
	}

	s.objects[key] = data

	s.writes++
	if s.lostAck {
		s.lostAck = false
		return "", errors.New("lost acknowledgement")
	}

	return "version-1", nil
}
func (s *memoryBackupStorage) Read(_ context.Context, key, _ string, offset int64, count int) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, ok := s.objects[key]
	if !ok || offset < 0 || offset+int64(count) > int64(len(data)) {
		return nil, errors.New("missing range")
	}

	return bytes.Clone(data[offset : offset+int64(count)]), nil
}
func (*memoryBackupStorage) RestoreURL(context.Context, string, string) (string, error) {
	return "https://objects.example/backup?signature=secret", nil
}
func (s *memoryBackupStorage) Delete(_ context.Context, key, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.failDelete {
		return errors.New("delete unavailable")
	}

	delete(s.objects, key)

	return nil
}

type backupTestRouter struct {
	*transferRouter
	offline bool
}

func (r *backupTestRouter) Files(ctx context.Context, id string, q model.FileRequest) (model.FileResult, error) {
	if r.offline {
		return model.FileResult{}, errors.New("spark offline")
	}

	return r.stores[id].Files(ctx, id, q)
}
func (r *backupTestRouter) Action(ctx context.Context, id string, q model.RuntimeRequest) (model.RuntimeStatus, error) {
	if r.offline {
		return model.RuntimeStatus{}, errors.New("spark offline")
	}

	return r.transferRouter.Action(ctx, id, q)
}

func backupFixture(t *testing.T) (*transferFixture, *BackupService, *memoryBackupStorage, *backupTestRouter) {
	t.Helper()

	f := newTransferFixture(t)
	if err := os.Remove(filepath.Join(f.source.Directory, f.server.ID, "backups", "nightly.tar.gz")); err != nil {
		t.Fatal(err)
	}

	router := &backupTestRouter{transferRouter: f.router}
	f.service.Deployment.Remote = router
	remote := &memoryBackupStorage{objects: make(map[string][]byte)}
	s := NewBackupService(repository.NewBackupRepository(f.db.ORM()), f.service.Deployment, remote)
	f.service.Deployment.Backups = s

	return f, s, remote, router
}

func seedBackup(t *testing.T, f *transferFixture, name string, at time.Time) {
	t.Helper()

	path := filepath.Join(f.source.Directory, f.server.ID, "backups", name+".tar.gz")
	if err := os.WriteFile(path, f.world, 0600); err != nil {
		t.Fatal(err)
	}

	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
}

func TestBackupIndependentRetentionAndOfflineDownload(t *testing.T) {
	f, s, remote, router := backupFixture(t)
	ctx := context.Background()

	now := time.Now()
	for i, name := range []string{"one", "two", "three", "four"} {
		seedBackup(t, f, name, now.Add(time.Duration(i)*time.Second))
	}

	p := model.BackupPolicy{Storage: "s3", LocalKeepLast: 3, RemoteKeepLast: 1}
	if err := s.SavePolicy(ctx, f.server.ID, p); err != nil {
		t.Fatal(err)
	}

	if err := s.reconcile(ctx, f.server.ID); err != nil {
		t.Fatal(err)
	}

	rows, err := s.Store.List(ctx, f.server.ID)
	if err != nil || len(rows) != 3 || len(remote.objects) != 1 {
		t.Fatal("independent counts", len(rows), len(remote.objects), err)
	}

	if _, err := os.Stat(filepath.Join(f.source.Directory, f.server.ID, "backups", "one.tar.gz")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("local retention not applied")
	}

	seedBackup(t, f, "five", now.Add(5*time.Second))

	if err := s.reconcile(ctx, f.server.ID); err != nil {
		t.Fatal(err)
	}

	if len(remote.objects) != 1 {
		t.Fatal("old S3 copy retained past limit")
	}

	remote.fail = true

	seedBackup(t, f, "six", now.Add(6*time.Second))

	if err := s.reconcile(ctx, f.server.ID); err != nil {
		t.Fatal(err)
	}

	if len(remote.objects) != 1 {
		t.Fatal("failed upload removed the previous remote backup")
	}

	p.LocalKeepLast = 1
	if err := s.SavePolicy(ctx, f.server.ID, p); err != nil {
		t.Fatal(err)
	}

	if err := s.reconcile(ctx, f.server.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(f.source.Directory, f.server.ID, "backups", "six.tar.gz")); err != nil {
		t.Fatal("pending upload lost local bytes", err)
	}

	router.offline = true

	items, err := s.List(ctx, f.server.ID)
	if err != nil || len(items) == 0 {
		t.Fatal("offline catalog hidden", err)
	}

	first, err := s.Read(ctx, f.server.ID, "five.tar.gz", 0)
	if err != nil || !bytes.Equal(first.Data, f.world[:model.FileLimit]) {
		t.Fatal("offline remote download failed", err)
	}

	last, err := s.Read(ctx, f.server.ID, "five.tar.gz", model.FileLimit)
	if err != nil || !bytes.Equal(last.Data, f.world[model.FileLimit:]) {
		t.Fatal("offline final chunk failed", err)
	}

	source, err := s.Source(ctx, f.server.ID, "five.tar.gz")
	if err != nil || source == nil || source.Size != int64(len(f.world)) {
		t.Fatal("remote restore descriptor missing", err)
	}
}

func TestBackupLostUploadAcknowledgementAndDurableDeletion(t *testing.T) {
	f, s, remote, _ := backupFixture(t)
	ctx := context.Background()

	seedBackup(t, f, "snapshot", time.Now())

	if err := s.SavePolicy(ctx, f.server.ID, model.BackupPolicy{Storage: "s3"}); err != nil {
		t.Fatal(err)
	}

	remote.lostAck = true

	if err := s.reconcile(ctx, f.server.ID); err != nil {
		t.Fatal(err)
	}

	row, err := s.Store.Get(ctx, f.server.ID, "snapshot.tar.gz")
	if err != nil || row.Remote || !row.Local || row.Error == "" {
		t.Fatal("lost ack not retained", row, err)
	}

	s = NewBackupService(s.Store, s.Deployment, remote)

	s.Deployment.Backups = s
	if err := s.reconcile(ctx, f.server.ID); err != nil {
		t.Fatal(err)
	}

	row, err = s.Store.Get(ctx, f.server.ID, "snapshot.tar.gz")
	if err != nil || !row.Remote || remote.writes != 1 {
		t.Fatal("restart duplicated upload", err)
	}

	remote.failDelete = true

	if err := s.Delete(ctx, f.server.ID, row.Name, "all"); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(f.source.Directory, f.server.ID, "backups", row.Name)); err != nil {
		t.Fatal("failed remote cleanup deleted local copy", err)
	}

	if _, err := s.Source(ctx, f.server.ID, row.Name); err == nil {
		t.Fatal("restored a deleting backup")
	}

	remote.failDelete = false
	s = NewBackupService(s.Store, s.Deployment, remote)

	s.Deployment.Backups = s
	if err := s.reconcile(ctx, f.server.ID); err != nil {
		t.Fatal(err)
	}

	rows, err := s.Store.List(ctx, f.server.ID)
	if err != nil || len(rows) != 0 || len(remote.objects) != 0 {
		t.Fatal("deletion did not resume", err)
	}

	if _, err := os.Stat(filepath.Join(f.source.Directory, f.server.ID, "data", "world.bin")); err != nil {
		t.Fatal("backup deletion touched game data", err)
	}
}

func TestBackupAgeRetentionAndExplicitCloudDeletion(t *testing.T) {
	f, s, remote, _ := backupFixture(t)
	ctx := context.Background()

	seedBackup(t, f, "old", time.Now().Add(-3*24*time.Hour))
	seedBackup(t, f, "recent", time.Now())

	p := model.BackupPolicy{Storage: "s3", LocalMaxAgeDays: 1, RemoteMaxAgeDays: 7}
	if err := s.SavePolicy(ctx, f.server.ID, p); err != nil {
		t.Fatal(err)
	}

	if err := s.reconcile(ctx, f.server.ID); err != nil {
		t.Fatal(err)
	}

	old, err := s.Store.Get(ctx, f.server.ID, "old.tar.gz")
	if err != nil || old.Local || !old.Remote {
		t.Fatal("independent age retention failed", err)
	}

	if err := s.Delete(ctx, f.server.ID, "recent.tar.gz", "s3"); err != nil {
		t.Fatal(err)
	}

	if err := s.reconcile(ctx, f.server.ID); err != nil {
		t.Fatal(err)
	}

	recent, err := s.Store.Get(ctx, f.server.ID, "recent.tar.gz")
	if err != nil || recent.Remote || !recent.Local {
		t.Fatal("explicit deletion was undone by sync", err)
	}

	p.RemoteMaxAgeDays = 1
	if err := s.SavePolicy(ctx, f.server.ID, p); err != nil {
		t.Fatal(err)
	}

	if err := s.reconcile(ctx, f.server.ID); err != nil {
		t.Fatal(err)
	}

	if len(remote.objects) != 0 {
		t.Fatal("remote age retention failed")
	}

	if _, err := s.Store.Get(ctx, f.server.ID, "old.tar.gz"); !errors.Is(err, repository.ErrAccessNotFound) {
		t.Fatal("expired metadata retained", err)
	}
}

func TestBackupCleanupOnServerDeletionAndTransferGuards(t *testing.T) {
	f, s, remote, _ := backupFixture(t)
	ctx := context.Background()

	seedBackup(t, f, "snapshot", time.Now())

	policy := model.BackupPolicy{Storage: "s3"}
	if err := s.SavePolicy(ctx, f.server.ID, policy); err != nil {
		t.Fatal(err)
	}

	if err := s.reconcile(ctx, f.server.ID); err != nil {
		t.Fatal(err)
	}

	transfer, err := f.service.Begin(ctx, f.server.ID, "target", f.ports)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.Delete(ctx, f.server.ID, "snapshot.tar.gz", "all"); !errors.Is(err, repository.ErrTransferActive) {
		t.Fatal("transfer allowed deletion", err)
	}

	if err := s.SavePolicy(ctx, f.server.ID, policy); !errors.Is(err, repository.ErrTransferActive) {
		t.Fatal("transfer allowed policy change", err)
	}

	if err := s.reconcile(ctx, f.server.ID); !errors.Is(err, repository.ErrTransferActive) {
		t.Fatal("transfer allowed retention", err)
	}

	if err := f.service.Store.Cancel(ctx, transfer.ID); err != nil {
		t.Fatal(err)
	}

	f.service.Start(ctx)
	waitTransfer(t, f.service, transfer.ID, "cancelled")
	f.service.Close()

	remote.failDelete = true

	if err := s.Deployment.Delete(ctx, f.server.ID); err == nil {
		t.Fatal("server deletion ignored failed cloud cleanup")
	}

	if _, err := s.Deployment.Servers.Get(ctx, f.server.ID); err != nil {
		t.Fatal("failed deletion removed server", err)
	}

	if _, err := os.Stat(filepath.Join(f.source.Directory, f.server.ID, "data", "world.bin")); err != nil {
		t.Fatal("failed deletion removed game data", err)
	}

	remote.failDelete = false

	if err := s.Deployment.Delete(ctx, f.server.ID); err != nil {
		t.Fatal(err)
	}

	if len(remote.objects) != 0 {
		t.Fatal("server deletion orphaned S3 backups")
	}

	rows, err := s.Store.List(ctx, f.server.ID)
	if err != nil || len(rows) != 0 {
		t.Fatal("server deletion retained backup catalog", err)
	}
}
