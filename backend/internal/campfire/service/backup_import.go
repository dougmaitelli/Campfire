package service

import (
	uploadstore "campfire/internal/shared/upload"
	"context"
	"crypto/rand"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"campfire/internal/shared/backuparchive"
	"campfire/internal/shared/model"
)

const backupUploadLifetime = time.Hour

// BackupImportTimeout bounds archive inspection and delivery to Spark.
const BackupImportTimeout = 30 * time.Minute
const backupUploadSessions = 8
const backupUploadsPerAccount = 2

// BackupStagingCopies bounds total reserved bytes independently of session count.
const BackupStagingCopies = 2

// BackupImports keeps bounded upload and preparation receipts owned by one account.
// Incomplete buffers are temporary. Finalized archives belong to the library and
// survive cancellation/expiry of a receipt or of the server-creation wizard.
type BackupImports struct {
	mu        sync.Mutex
	uploads   map[string]*BackupUpload
	directory string
	limit     int64
	reserved  int64
	initErr   error
}

// BackupUpload fields are accessed only inside With's locked callback.
type BackupUpload struct {
	buffer        *uploadstore.Session
	temporary     string
	BackupID      string
	OriginalName  string
	LibraryOnly   bool
	FromLibrary   bool
	source        func(context.Context, int64) (model.FileResult, error)
	owner         int64
	mu            sync.Mutex
	file          *os.File
	size          int64
	timer         *time.Timer
	Target        string
	Manifest      backuparchive.Manifest
	Name          string
	CreatedServer string
	published     bool
	closed        bool
	store         *BackupImports
}

// NewBackupImports creates account-bound sessions under a storage root. The CLI
// supplies its persistent state directory; an empty root is for embedded/test use.
func NewBackupImports(directory string, limit int64) *BackupImports {
	return &BackupImports{uploads: make(map[string]*BackupUpload), directory: directory, limit: limit}
}

func importError(message string) error {
	return &model.FileError{Code: model.FileInvalid, Message: message}
}

// Begin reserves one bounded upload, optionally bound to an existing server.
func (s *BackupImports) Begin(owner int64, target string, size, limit int64) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureDirectory(); err != nil {
		return "", err
	}

	count := 0

	active := make(map[string]bool, len(s.uploads))
	for _, upload := range s.uploads {
		active[upload.temporary] = true
		if upload.owner == owner {
			count++
		}
	}

	uploadstore.Prune(filepath.Join(s.directory, "uploads"), "upload-", time.Now().Add(-backupUploadLifetime), active)

	if size < 1 || size > limit {
		return "", importError("Backup exceeds the configured upload limit.")
	}

	if len(s.uploads) >= backupUploadSessions || count >= backupUploadsPerAccount {
		return "", importError("Too many backup uploads are active; cancel an upload or wait for it to expire.")
	}

	if size > s.limit-s.reserved {
		return "", importError("Backup upload staging is full; cancel an upload or wait for it to expire.")
	}

	buffer, err := uploadstore.Create(filepath.Join(s.directory, "uploads"), "upload-", size, limit)
	if err != nil {
		return "", err
	}

	token := rand.Text()
	upload := &BackupUpload{owner: owner, buffer: buffer, temporary: buffer.File.Name(), file: buffer.File, size: size, Target: target, store: s}
	s.uploads[token] = upload
	s.reserved += size
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

	if upload.closed {
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

		if upload.timer != nil {
			upload.timer.Stop()
		}

		upload.closed = true
		upload.releaseFile()

		return nil
	})
}

func (u *BackupUpload) releaseFile() {
	if u.file == nil {
		return
	}

	u.buffer.Close()
	u.file = nil
	u.store.mu.Lock()
	u.store.reserved -= u.size
	u.store.mu.Unlock()
}

// Chunk appends the next bounded range without allowing gaps or overwrites.
func (u *BackupUpload) Chunk(offset int64, data []byte) error {
	if u.file == nil || u.Manifest.Format != 0 || offset != u.buffer.Offset || len(data) > model.FileLimit || int64(len(data)) > u.size-u.buffer.Offset {
		return importError("Invalid backup upload offset or size.")
	}

	if err := u.buffer.Write(offset, data); err != nil {
		return importError("Could not store backup upload.")
	}

	return nil
}

// Inspect validates a complete archive before it can be previewed or published.
func (u *BackupUpload) Inspect(ctx context.Context) error {
	if u.Manifest.Format != 0 {
		return nil
	}

	if err := u.buffer.Complete(); err != nil {
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

	ctx, cancel := context.WithTimeout(ctx, BackupImportTimeout)
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

	limit := deployment.UploadLimit()
	if u.FromLibrary {
		limit = model.FileTransferLimit
	} // Existing archives are not browser uploads.

	request := model.FileRequest{Operation: model.FileOperationBackupUploadBegin, Path: u.Name, Size: u.size, UploadLimit: limit}

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

		if u.source != nil {
			chunk, err := u.source(ctx, request.Offset)
			if err != nil {
				return err
			}

			if chunk.Size != u.size || len(chunk.Data) == 0 || int64(len(chunk.Data)) > u.size-request.Offset {
				return importError("Backup changed during publication.")
			}

			request.Data = chunk.Data
		} else {
			request.Data = make([]byte, min(int64(model.FileLimit), u.size-request.Offset))
			if _, err := u.file.ReadAt(request.Data, request.Offset); err != nil {
				return err
			}
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
	u.releaseFile()

	return nil
}
