package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"sync"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

// BackupStorage holds off-host bytes; credentials are never sent to a Spark.
type BackupStorage interface {
	ID() string
	Put(context.Context, string, int64, string, io.Reader) (string, error)
	Read(context.Context, string, string, int64, int) ([]byte, error)
	RestoreURL(context.Context, string, string) (string, error)
	Delete(context.Context, string, string) error
}

// BackupService catalogs completed archives and reconciles independent local
// and off-host retention. One bounded worker runs per server, at most two total.
type BackupService struct {
	Store      *repository.BackupRepository
	Deployment *DeploymentService
	Remote     BackupStorage
	mu         sync.Mutex
	active     map[string]bool
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	cursor     int
}

// NewBackupService creates the catalog and optional S3 reconciliation service.
func NewBackupService(store *repository.BackupRepository, deployment *DeploymentService, remote BackupStorage) *BackupService {
	return &BackupService{Store: store, Deployment: deployment, Remote: remote, active: make(map[string]bool)}
}

func (s *BackupService) acquire(id string) (func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.active[id] {
		return nil, &model.FileError{Code: "file_busy", Message: "Wait for backup synchronization or deletion to finish."}
	}

	s.active[id] = true

	return func() { s.mu.Lock(); delete(s.active, id); s.mu.Unlock() }, nil
}

// Start scans all servers so manual and scheduled backups share the same sync path.
func (s *BackupService) Start(ctx context.Context) {
	ctx, s.cancel = context.WithCancel(ctx)

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		for {
			servers, err := s.Deployment.Servers.List(ctx)
			if err == nil {
				for attempt := 0; attempt < len(servers); attempt++ {
					s.mu.Lock()
					full := len(s.active) >= 2
					s.mu.Unlock()

					if full {
						break
					}

					server := servers[s.cursor%len(servers)]
					s.cursor++

					release, err := s.acquire(server.ID)
					if err != nil {
						continue
					}

					s.wg.Add(1)
					go func() {
						defer s.wg.Done()
						defer release()

						work, cancel := context.WithTimeout(ctx, 45*time.Minute)
						defer cancel()

						if err := s.reconcile(work, server.ID); err != nil && work.Err() == nil {
							slog.Debug("Backup reconciliation deferred", "server", server.ID)
						}
					}()
				}
			}

			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

// Close leaves incomplete upload/deletion checkpoints for the next process.
func (s *BackupService) Close() {
	if s.cancel != nil {
		s.cancel()
		s.wg.Wait()
	}
}

// SavePolicy validates opt-in limits. Zero means unlimited for that dimension.
func (s *BackupService) SavePolicy(ctx context.Context, id string, p model.BackupPolicy) error {
	if (p.Storage != "local" && p.Storage != "s3") || p.LocalKeepLast < 0 || p.LocalKeepLast > 1000 || p.RemoteKeepLast < 0 || p.RemoteKeepLast > 1000 || p.LocalMaxAgeDays < 0 || p.LocalMaxAgeDays > 36500 || p.RemoteMaxAgeDays < 0 || p.RemoteMaxAgeDays > 36500 {
		return repository.ErrAccessInput
	}

	if p.Storage == "s3" && s.Remote == nil {
		return &model.FileError{Code: "invalid_file", Message: "S3 backup storage is not configured."}
	}

	return s.Store.SavePolicy(ctx, id, p)
}

func (s *BackupService) discover(ctx context.Context, id string) error {
	result, err := s.Deployment.Files(ctx, id, model.FileRequest{Operation: "backup.list", Path: "."})
	if err != nil {
		return err
	}

	seen := make(map[string]bool)

	for _, item := range result.Entries {
		if !model.BackupName(item.Name) || item.Size < 1 || item.Size > 100<<30 {
			continue
		}

		seen[item.Name] = true
		if err := s.Store.Discover(ctx, database.Backup{ServerID: id, Name: item.Name, Size: item.Size, CreatedAt: item.ModifiedAt.UnixNano(), Local: true, Key: id + "/" + rand.Text() + "/" + item.Name}); err != nil {
			return err
		}
	}

	rows, err := s.Store.List(ctx, id)
	if err != nil {
		return err
	}

	for _, row := range rows {
		if row.Local != seen[row.Name] {
			if err := s.Store.Update(ctx, id, row.Name, map[string]any{"local": seen[row.Name]}); err != nil {
				return err
			}
		}
	}

	return nil
}

// List combines durable off-host entries with newly discovered local archives.
// A disconnected Spark does not hide the off-host catalog.
func (s *BackupService) List(ctx context.Context, id string) ([]model.Backup, error) {
	if _, err := s.Deployment.Servers.Get(ctx, id); err != nil {
		return nil, err
	}

	var discoverErr error
	if release, err := s.acquire(id); err == nil {
		discoverErr = s.discover(ctx, id)

		release()
	}

	rows, err := s.Store.List(ctx, id)
	if err != nil {
		return nil, err
	}

	var failure *model.FileError

	busy := errors.As(discoverErr, &failure) && failure.Code == "file_busy"
	if discoverErr != nil && len(rows) == 0 && !busy {
		return nil, discoverErr
	}

	s.mu.Lock()
	working := s.active[id]
	s.mu.Unlock()

	items := make([]model.Backup, 0, len(rows))
	for _, row := range rows {
		items = append(items, backupView(row, working))
	}

	return items, nil
}

func backupView(row database.Backup, working bool) model.Backup {
	state := "available"
	if !row.Local && !row.Remote {
		state = "unavailable"
	}

	if row.Error != "" {
		state = "error"
	}

	if working && !row.Remote && row.Local {
		state = "syncing"
	}

	if row.Deleting || row.LocalDeleting || row.RemoteDeleting {
		state = "deleting"
	}

	return model.Backup{Name: row.Name, Size: row.Size, ModifiedAt: time.Unix(0, row.CreatedAt).UTC(), Local: row.Local, Remote: row.Remote, State: state, Error: row.Error}
}

// Catalog lists known backups across servers, including disconnected placements.
func (s *BackupService) Catalog(ctx context.Context) ([]model.CatalogBackup, error) {
	rows, err := s.Store.ListAll(ctx)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	items := make([]model.CatalogBackup, 0, len(rows))
	for _, row := range rows {
		items = append(items, model.CatalogBackup{Backup: backupView(row.Backup, s.active[row.ServerID]), ServerID: row.ServerID, ServerName: row.ServerName})
	}

	return items, nil
}

func (s *BackupService) storage(row database.Backup) (BackupStorage, error) {
	if s.Remote == nil || row.StoreID != s.Remote.ID() {
		return nil, &model.FileError{Code: "file_io", Message: "This backup's S3 storage configuration is unavailable."}
	}

	return s.Remote, nil
}

// Read serves local or remote bounded chunks through the same authenticated API.
func (s *BackupService) Read(ctx context.Context, id, name string, offset int64) (model.FileResult, error) {
	if !model.BackupName(name) || offset < 0 {
		return model.FileResult{}, repository.ErrAccessInput
	}

	row, err := s.Store.Get(ctx, id, name)
	if errors.Is(err, repository.ErrAccessNotFound) {
		return s.Deployment.Files(ctx, id, model.FileRequest{Operation: "backup.read", Path: name, Offset: offset})
	}

	if err != nil {
		return model.FileResult{}, err
	}

	if row.Deleting || row.LocalDeleting || row.RemoteDeleting {
		return model.FileResult{}, repository.ErrAccessConflict
	}

	if !row.Remote {
		return s.Deployment.Files(ctx, id, model.FileRequest{Operation: "backup.read", Path: name, Offset: offset})
	}

	remote, err := s.storage(row)
	if err != nil {
		return model.FileResult{}, err
	}

	if offset >= row.Size {
		return model.FileResult{}, repository.ErrAccessInput
	}

	data, err := remote.Read(ctx, row.Key, row.Version, offset, int(min(int64(model.FileLimit), row.Size-offset)))

	return model.FileResult{ServerID: id, Path: name, Size: row.Size, Revision: row.Digest, Data: data}, err
}

// Source issues a fresh object-specific URL only when restoration needs S3 bytes.
func (s *BackupService) Source(ctx context.Context, id, name string) (*model.BackupSource, error) {
	if !model.BackupName(name) {
		return nil, repository.ErrAccessInput
	}

	row, err := s.Store.Get(ctx, id, name)
	if errors.Is(err, repository.ErrAccessNotFound) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	if row.Deleting || row.LocalDeleting || row.RemoteDeleting {
		return nil, repository.ErrAccessConflict
	}

	if row.Local || !row.Remote {
		return nil, nil
	}

	remote, err := s.storage(row)
	if err != nil {
		return nil, err
	}

	address, err := remote.RestoreURL(ctx, row.Key, row.Version)
	if err != nil {
		return nil, ErrDeploymentUnavailable
	}

	return &model.BackupSource{URL: address, Size: row.Size, Digest: row.Digest}, nil
}

type backupStream struct {
	s            *BackupService
	ctx          context.Context
	id, name     string
	offset, size int64
	buffer       []byte
}

func (r *backupStream) Read(p []byte) (int, error) {
	if len(r.buffer) == 0 {
		if r.offset == r.size {
			return 0, io.EOF
		}

		chunk, err := r.s.Deployment.Files(r.ctx, r.id, model.FileRequest{Operation: "backup.read", Path: r.name, Offset: r.offset})
		if err != nil {
			return 0, err
		}

		if chunk.Size != r.size || len(chunk.Data) == 0 || int64(len(chunk.Data)) > r.size-r.offset {
			return 0, errors.New("backup changed during synchronization")
		}

		r.buffer = chunk.Data
	}

	n := copy(p, r.buffer)
	r.buffer = r.buffer[n:]
	r.offset += int64(n)

	return n, nil
}

func (s *BackupService) upload(ctx context.Context, row database.Backup) error {
	if s.Remote == nil {
		return ErrDeploymentUnavailable
	}

	if row.StoreID != "" && row.StoreID != s.Remote.ID() {
		return errors.New("backup storage configuration changed")
	}

	hash := sha256.New()
	if _, err := io.Copy(hash, &backupStream{s: s, ctx: ctx, id: row.ServerID, name: row.Name, size: row.Size}); err != nil {
		return err
	}

	digest := hex.EncodeToString(hash.Sum(nil))
	if row.Digest != "" && row.Digest != digest {
		return errors.New("backup checksum changed")
	}

	if err := s.Store.Update(ctx, row.ServerID, row.Name, map[string]any{"store_id": s.Remote.ID(), "digest": digest}); err != nil {
		return err
	}

	version, err := s.Remote.Put(ctx, row.Key, row.Size, digest, &backupStream{s: s, ctx: ctx, id: row.ServerID, name: row.Name, size: row.Size})
	if err != nil {
		return err
	}

	return s.Store.Update(ctx, row.ServerID, row.Name, map[string]any{"remote": true, "version": version, "error": ""})
}
