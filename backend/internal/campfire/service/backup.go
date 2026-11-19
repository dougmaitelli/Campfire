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
	"campfire/internal/shared/limits"
	"campfire/internal/shared/model"
	"campfire/internal/shared/workprogress"
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
	Store            *repository.BackupRepository
	Deployment       *DeploymentService
	Remote           BackupStorage
	LibraryDirectory string
	mu               sync.Mutex
	active           map[string]bool
	uploading        map[string]string
	progress         map[string]workprogress.Snapshot
	cancel           context.CancelFunc
	wg               sync.WaitGroup
	cursor           int
}

// NewBackupService creates the catalog and optional S3 reconciliation service.
func NewBackupService(store *repository.BackupRepository, deployment *DeploymentService, remote BackupStorage) *BackupService {
	return &BackupService{Store: store, Deployment: deployment, Remote: remote, active: make(map[string]bool), uploading: make(map[string]string)}
}

func (s *BackupService) acquire(id string) (func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.active[id] {
		return nil, &model.FileError{Code: model.FileBusy, Message: "Wait for backup synchronization or deletion to finish."}
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
	if err := s.validatePolicy(p); err != nil {
		return err
	}

	return s.Store.SavePolicy(ctx, id, p)
}

func (s *BackupService) validatePolicy(p model.BackupPolicy) error {
	if p.Storage != model.BackupStorageLocal && p.Storage != model.BackupStorageS3 {
		return repository.ErrAccessInput
	}

	for _, mode := range []model.BackupMode{model.BackupModeFull, model.BackupModeSavedata} {
		retention := p.Retention(mode)
		if retention.LocalKeepLast < 0 || retention.LocalKeepLast > limits.BackupRetentionCopies || retention.RemoteKeepLast < 0 || retention.RemoteKeepLast > limits.BackupRetentionCopies || retention.LocalMaxAgeDays < 0 || retention.LocalMaxAgeDays > limits.BackupRetentionDays || retention.RemoteMaxAgeDays < 0 || retention.RemoteMaxAgeDays > limits.BackupRetentionDays {
			return repository.ErrAccessInput
		}
	}

	if p.Storage == model.BackupStorageS3 && s.Remote == nil {
		return &model.FileError{Code: model.FileInvalid, Message: "S3 backup storage is not configured."}
	}

	return nil
}

func (s *BackupService) discover(ctx context.Context, id string) error {
	result, err := s.Deployment.Files(ctx, id, model.FileRequest{Operation: model.FileOperationBackupList, Path: "."})
	if err != nil {
		return err
	}

	return s.catalogLocal(ctx, id, result)
}

func (s *BackupService) catalogLocal(ctx context.Context, id string, result model.FileResult) error {
	server, err := s.Deployment.Servers.Get(ctx, id)
	if err != nil {
		return err
	}

	seen := make(map[string]bool)
	temporary := make(map[string]bool)

	if s.Deployment.Copies != nil {
		jobs, err := s.Deployment.Copies.Store.Jobs().ActiveForServer(ctx, id, model.JobCopy)
		if err != nil {
			return err
		}

		for _, job := range jobs {
			temporary[model.CopyArchiveName(job.ID)] = true
		}
	}

	for _, item := range result.Entries {
		if temporary[item.Name] {
			continue
		}

		if !model.BackupName(item.Name) || item.Size < 1 || item.Size > 100<<30 {
			continue
		}

		seen[item.Name] = true
		if err := s.Store.Discover(ctx, database.Backup{Portable: item.Portable, Mode: item.BackupMode.Normalized(), ServerID: id, LocalStore: "spark:" + server.SparkID, LocalKey: id + "/" + item.Name, Name: item.Name, Size: item.Size, CreatedAt: item.ModifiedAt.UnixNano(), Local: true, Key: id + "/" + rand.Text() + "/" + item.Name}); err != nil {
			return err
		}
	}

	rows, err := s.Store.List(ctx, id)
	if err != nil {
		return err
	}

	for _, row := range rows {
		if row.Local != seen[row.Name] || seen[row.Name] && row.LocalStore != "spark:"+server.SparkID {
			if err := s.Store.Update(ctx, id, row.Name, map[string]any{"local": seen[row.Name], "local_store": "spark:" + server.SparkID, "local_key": id + "/" + row.Name}); err != nil {
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

	busy := errors.As(discoverErr, &failure) && failure.Code == model.FileBusy
	if discoverErr != nil && len(rows) == 0 && !busy {
		return nil, discoverErr
	}

	s.mu.Lock()

	uploading := s.uploading[id]
	defer s.mu.Unlock()

	items := make([]model.Backup, 0, len(rows))
	for _, row := range rows {
		items = append(items, s.progressView(row, uploading == row.Name))
	}

	return items, nil
}

func backupView(row database.Backup, uploading bool) model.Backup {
	state := model.BackupAvailable
	if !row.Local && !row.Remote {
		state = model.BackupUnavailable
	}

	if row.Error != "" {
		state = model.BackupError
	}

	if uploading && !row.Remote && row.Local {
		state = model.BackupSyncing
	}

	if row.Deleting || row.LocalDeleting || row.RemoteDeleting {
		state = model.BackupDeleting
	}

	return model.Backup{ID: row.ID, DisplayName: row.DisplayName, Pinned: row.Pinned, Portable: row.Portable, Mode: row.Mode.Normalized(), Name: row.Name, Size: row.Size, ModifiedAt: time.Unix(0, row.CreatedAt).UTC(), Local: row.Local, Remote: row.Remote, State: state, Error: row.Error}
}

// progressView requires s.mu. Only active synchronization exposes transient
// counters; terminal availability still comes exclusively from durable copy state.
func (s *BackupService) progressView(row database.Backup, uploading bool) model.Backup {
	view := backupView(row, uploading)
	if view.State == model.BackupSyncing {
		if progress, ok := s.progress[row.ID]; ok {
			view.WorkProgress = &progress
		}
	}

	return view
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
		items = append(items, model.CatalogBackup{Backup: s.progressView(row.Backup, s.uploading[row.ServerID] == row.Name), OwnerUserID: row.OwnerUserID, ServerID: row.ServerID, ServerName: row.ServerName, LocalSparkID: row.LocalSparkID, LocalSparkName: row.LocalSparkName})
	}

	return items, nil
}

func (s *BackupService) storage(row database.Backup) (BackupStorage, error) {
	if s.Remote == nil || row.StoreID != s.Remote.ID() {
		return nil, &model.FileError{Code: model.FileIO, Message: "This backup's S3 storage configuration is unavailable."}
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
		return s.Deployment.Files(ctx, id, model.FileRequest{Operation: model.FileOperationBackupRead, Path: name, Offset: offset})
	}

	if err != nil {
		return model.FileResult{}, err
	}

	return s.readArchive(ctx, row, offset)
}

func (s *BackupService) readArchive(ctx context.Context, row database.Backup, offset int64) (model.FileResult, error) {
	if row.Deleting || row.LocalDeleting || row.RemoteDeleting {
		return model.FileResult{}, repository.ErrAccessConflict
	}

	if !row.Remote {
		if row.LocalStore == "panel" {
			return s.readLibraryFile(row, offset)
		}

		return s.localArchiveFiles(ctx, row, model.FileOperationBackupRead, offset)
	}

	remote, err := s.storage(row)
	if err != nil {
		return model.FileResult{}, err
	}

	if offset >= row.Size {
		return model.FileResult{}, repository.ErrAccessInput
	}

	data, err := remote.Read(ctx, row.Key, row.Version, offset, int(min(int64(model.FileLimit), row.Size-offset)))

	return model.FileResult{ServerID: row.ServerID, Path: row.Name, Size: row.Size, Revision: row.Digest, Data: data}, err
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

func (s *BackupService) upload(ctx context.Context, row database.Backup) error {
	if s.Remote == nil {
		return ErrDeploymentUnavailable
	}

	if row.StoreID != "" && row.StoreID != s.Remote.ID() {
		return errors.New("backup storage configuration changed")
	}

	s.mu.Lock()
	s.uploading[row.ServerID] = row.Name
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.uploading, row.ServerID)
		delete(s.progress, row.ID)
		s.mu.Unlock()
	}()

	ctx = workprogress.WithReporter(ctx, func(progress workprogress.Snapshot) {
		s.mu.Lock()
		defer s.mu.Unlock()

		if s.progress == nil {
			s.progress = make(map[string]workprogress.Snapshot)
		}

		s.progress[row.ID] = progress
	})

	newReader := func() io.Reader {
		return &backupChunkReader{ctx: ctx, size: row.Size, read: func(ctx context.Context, offset int64) (model.FileResult, error) {
			return s.Deployment.Files(ctx, row.ServerID, model.FileRequest{Operation: model.FileOperationBackupRead, Path: row.Name, Offset: offset})
		}}
	}

	hash := sha256.New()
	validation := workprogress.New(ctx, workprogress.Validating, row.Size)
	_, hashErr := io.Copy(hash, validation.Reader(newReader()))
	validation.Flush()

	if hashErr != nil {
		return hashErr
	}

	digest := hex.EncodeToString(hash.Sum(nil))
	if row.Digest != "" && row.Digest != digest {
		return errors.New("backup checksum changed")
	}

	if err := s.Store.Update(ctx, row.ServerID, row.Name, map[string]any{"store_id": s.Remote.ID(), "digest": digest}); err != nil {
		return err
	}

	upload := workprogress.New(ctx, workprogress.Uploading, row.Size)
	version, err := s.Remote.Put(ctx, row.Key, row.Size, digest, upload.Reader(newReader()))
	upload.Flush()

	if err != nil {
		return err
	}

	workprogress.New(ctx, workprogress.Committing, 0)

	return s.Store.Update(ctx, row.ServerID, row.Name, map[string]any{"remote": true, "version": version, "error": ""})
}
