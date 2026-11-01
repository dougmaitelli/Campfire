package service

import (
	"context"
	"crypto/rand"
	"io"
	"os"
	"sync"
	"time"

	"campfire/internal/shared/backuparchive"
	"campfire/internal/shared/model"
)

const backupUploadLifetime = time.Hour

// BackupImports keeps private, bounded upload sessions owned by one account.
// Archives are temporary and are removed on cancellation, publication or expiry.
type BackupImports struct {
	mu      sync.Mutex
	uploads map[string]*BackupUpload
}

// BackupUpload fields are accessed only inside With's locked callback.
type BackupUpload struct {
	owner         int64
	mu            sync.Mutex
	file          *os.File
	size, offset  int64
	timer         *time.Timer
	Target        string
	Manifest      backuparchive.Manifest
	Name          string
	CreatedServer string
	published     bool
}

// NewBackupImports creates an account-bound temporary archive store.
func NewBackupImports() *BackupImports {
	return &BackupImports{uploads: make(map[string]*BackupUpload)}
}

func importError(message string) error {
	return &model.FileError{Code: model.FileInvalid, Message: message}
}

// Begin reserves one bounded upload, optionally bound to an existing server.
func (s *BackupImports) Begin(owner int64, target string, size, limit int64) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := 0

	for _, upload := range s.uploads {
		if upload.owner == owner {
			count++
		}
	}

	if size < 1 || size > limit {
		return "", importError("Backup exceeds the configured upload limit.")
	}

	if len(s.uploads) >= 8 || count >= 2 {
		return "", importError("Too many backup uploads are active; cancel an upload or wait for it to expire.")
	}

	file, err := os.CreateTemp("", "campfire-backup-import-")
	if err != nil {
		return "", err
	}
	// Keep only the private descriptor, so crashes cannot leave credential-bearing
	// archives behind in the panel's temporary directory.
	if err := os.Remove(file.Name()); err != nil {
		_ = file.Close()
		return "", err
	}

	token := rand.Text()
	upload := &BackupUpload{owner: owner, file: file, size: size, Target: target}
	s.uploads[token] = upload
	upload.timer = time.AfterFunc(backupUploadLifetime, func() { s.Cancel(owner, token) })

	return token, nil
}

// With prevents expiry, cancellation and competing requests from changing a
// session while it is being validated, bound to a new server or published.
func (s *BackupImports) With(owner int64, token string, use func(*BackupUpload) error) error {
	s.mu.Lock()
	upload := s.uploads[token]
	s.mu.Unlock()

	if upload == nil || upload.owner != owner {
		return &model.FileError{Code: model.FileNotFound, Message: "Backup upload expired or is unavailable."}
	}

	upload.mu.Lock()
	defer upload.mu.Unlock()

	if upload.file == nil {
		return importError("Backup upload is unavailable.")
	}

	return use(upload)
}

// Cancel closes the private descriptor and removes the session.
func (s *BackupImports) Cancel(owner int64, token string) {
	_ = s.With(owner, token, func(upload *BackupUpload) error {
		s.mu.Lock()
		delete(s.uploads, token)
		s.mu.Unlock()
		upload.timer.Stop()
		_ = upload.file.Close()
		_ = os.Remove(upload.file.Name())
		upload.file = nil

		return nil
	})
}

// Chunk appends the next bounded range without allowing gaps or overwrites.
func (u *BackupUpload) Chunk(offset int64, data []byte) error {
	if u.Manifest.Format != 0 || offset != u.offset || len(data) > model.FileLimit || int64(len(data)) > u.size-u.offset {
		return importError("Invalid backup upload offset or size.")
	}

	n, err := u.file.WriteAt(data, offset)
	if err != nil || n != len(data) {
		return importError("Could not store backup upload.")
	}

	u.offset += int64(n)

	return nil
}

// Inspect validates a complete archive before it can be previewed or published.
func (u *BackupUpload) Inspect(ctx context.Context) error {
	if u.Manifest.Format != 0 {
		return nil
	}

	if u.offset != u.size {
		return importError("Backup upload is incomplete.")
	}

	if _, err := u.file.Seek(0, io.SeekStart); err != nil {
		return err
	}

	manifest, err := backuparchive.Inspect(ctx, u.file)
	if err != nil {
		return importError(err.Error())
	}

	u.Manifest = manifest
	u.Name = manifest.Mode.ArchiveName(rand.Text())

	return nil
}

// Publish copies validated bytes into Spark's isolated immutable backup store.
// Holding the delivery lock also prevents placement or configuration changes.
func (u *BackupUpload) Publish(ctx context.Context, deployment *DeploymentService, id string, authorize func() error) error {
	if u.Target != id || u.Manifest.Format == 0 {
		return importError("Backup upload does not belong to this server.")
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	if deployment.Backups != nil {
		finish, err := deployment.Backups.acquire(id)
		if err != nil {
			return err
		}
		defer finish()
	}

	release, err := deployment.lockServer(ctx, id)
	if err != nil {
		return err
	}

	err = u.publishBytes(ctx, deployment, id, authorize)

	release()

	if err != nil {
		return err
	}

	if deployment.Backups != nil {
		return deployment.Backups.discover(ctx, id)
	}

	return nil
}

func (u *BackupUpload) publishBytes(ctx context.Context, deployment *DeploymentService, id string, authorize func() error) error {
	if err := authorize(); err != nil {
		return err
	}

	if u.published {
		return nil
	}

	if _, err := deployment.sync(ctx, id); err != nil {
		return err
	}

	request := model.FileRequest{Operation: model.FileOperationBackupUploadBegin, Path: u.Name, Size: u.size, UploadLimit: deployment.UploadLimit()}

	result, err := deployment.filesUnlocked(ctx, id, request)
	if err != nil {
		return err
	}

	request.UploadID = result.UploadID

	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()

		request.Operation = model.FileOperationBackupUploadCancel
		request.Data = nil
		_, _ = deployment.filesUnlocked(cleanup, id, request)
	}()

	request.Operation = model.FileOperationBackupUploadChunk
	for request.Offset < u.size {
		if err := authorize(); err != nil {
			return err
		}

		request.Data = make([]byte, min(int64(model.FileLimit), u.size-request.Offset))
		if _, err := u.file.ReadAt(request.Data, request.Offset); err != nil {
			return err
		}

		if _, err := deployment.filesUnlocked(ctx, id, request); err != nil {
			return err
		}

		request.Offset += int64(len(request.Data))
	}

	request.Operation = model.FileOperationBackupUploadCommit
	request.Data = nil

	if err := authorize(); err != nil {
		return err
	}

	_, err = deployment.filesUnlocked(ctx, id, request)
	if err != nil {
		return err
	}

	u.published = true

	return nil
}
