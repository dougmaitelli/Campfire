package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
	"campfire/internal/shared/utils"
)

// ArchiveDirectory is durable storage, separate from the common upload area.
func (s *BackupImports) ArchiveDirectory() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ensureDirectory() != nil {
		return ""
	}

	return filepath.Join(s.directory, "backups")
}

// The default is for embedded/test use; the CLI supplies its persistent state
// directory. Called with the session-store lock held.
func (s *BackupImports) ensureDirectory() error {
	if s.directory == "" && s.initErr == nil {
		s.directory, s.initErr = os.MkdirTemp("", "campfire-storage-")
	}

	return s.initErr
}

// Store finalizes a validated upload as an independent archive. Session expiry
// and cancelling the server wizard remove only temporary upload state.
func (u *BackupUpload) Store(ctx context.Context, backups *BackupService) (database.Backup, error) {
	if backups == nil || u.Manifest.Format == 0 || u.Target != "" {
		return database.Backup{}, ErrDeploymentUnavailable
	}

	if u.BackupID != "" {
		row, err := backups.Store.ByID(ctx, u.BackupID)
		if err == nil || !errors.Is(err, repository.ErrAccessNotFound) {
			if err == nil && u.file != nil {
				u.useStoredArchive(backups, row.ID)
			}

			return row, err
		}
	} else {
		u.BackupID = rand.Text()
	}

	if u.file == nil {
		return database.Backup{}, importError("Backup upload is unavailable.")
	}

	directory := backups.LibraryDirectory
	if directory == "" {
		return database.Backup{}, ErrDeploymentUnavailable
	}

	if err := os.MkdirAll(directory, 0700); err != nil {
		return database.Backup{}, err
	}

	hash := sha256.New()
	if _, err := io.Copy(hash, io.NewSectionReader(u.file, 0, u.size)); err != nil {
		return database.Backup{}, err
	}

	if err := ctx.Err(); err != nil {
		return database.Backup{}, err
	}

	key := u.BackupID + ".tar.gz"
	if err := u.buffer.PublishNoReplace(filepath.Join(directory, key)); err != nil {
		return database.Backup{}, err
	}

	owner := u.owner

	row := database.Backup{ID: u.BackupID, OwnerUserID: &owner, Name: u.Name, DisplayName: u.OriginalName, Size: u.size, CreatedAt: time.Now().UnixNano(), Local: true, LocalStore: "panel", LocalKey: key, Portable: true, Mode: u.Manifest.Mode, Digest: hex.EncodeToString(hash.Sum(nil)), Key: u.BackupID + "/" + u.Name}
	// Once the complete archive is durable, finish its catalog registration even
	// if the browser closes. Cancellation must not strand a successfully stored
	// archive between the filesystem and database. Keep bytes on ambiguous errors.
	registration, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()

	if err := backups.Store.SaveArchive(registration, row); err != nil {
		return database.Backup{}, err
	}

	u.useStoredArchive(backups, row.ID)

	return row, nil
}

func (u *BackupUpload) useStoredArchive(backups *BackupService, id string) {
	u.source = func(ctx context.Context, offset int64) (model.FileResult, error) {
		return backups.ReadArchive(ctx, id, offset)
	}
	u.releaseFile()
}

// ReadArchive reads by stable archive identity, with no parent-server dependency.
func (s *BackupService) ReadArchive(ctx context.Context, id string, offset int64) (model.FileResult, error) {
	row, err := s.Store.ByID(ctx, id)
	if err != nil {
		return model.FileResult{}, err
	}

	if offset < 0 || offset >= row.Size {
		return model.FileResult{}, repository.ErrAccessInput
	}

	return s.readArchive(ctx, row, offset)
}

func (s *BackupService) readLibraryFile(row database.Backup, offset int64) (model.FileResult, error) {
	result := model.FileResult{Path: row.Name, Size: row.Size, Revision: row.Digest}
	if row.LocalKey != row.ID+".tar.gz" || s.LibraryDirectory == "" || offset < 0 || offset >= row.Size {
		return result, repository.ErrAccessInput
	}

	root, err := os.OpenRoot(s.LibraryDirectory)
	if err != nil {
		return result, err
	}

	defer func() { _ = root.Close() }()

	file, err := root.OpenFile(row.LocalKey, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return result, err
	}

	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != row.Size {
		return result, importError("Backup storage changed.")
	}

	result.Data = make([]byte, min(int64(model.FileLimit), row.Size-offset))
	_, err = file.ReadAt(result.Data, offset)

	return result, err
}

func (s *BackupService) localArchiveFiles(ctx context.Context, row database.Backup, operation model.FileOperation, offset int64) (model.FileResult, error) {
	if row.ServerID != "" {
		return s.Deployment.Files(ctx, row.ServerID, model.FileRequest{Operation: operation, Path: row.Name, Offset: offset})
	}
	// This is a storage locator only. It never grants former members access or
	// appears as the archive's parent/source identity in the public catalog.
	sparkID, ok := strings.CutPrefix(row.LocalStore, "spark:")

	namespace, name, found := strings.Cut(row.LocalKey, "/")
	if !ok || !found || !utils.IsValidResourceID(namespace) || !model.BackupName(name) {
		return model.FileResult{}, &model.FileError{Code: model.FileIO, Message: "The local archive's storage locator is unavailable. Its bytes have not been removed."}
	}

	node, err := s.Deployment.Sparks.Get(ctx, sparkID)
	if err != nil || node.Disabled {
		return model.FileResult{}, ErrDeploymentUnavailable
	}

	files, ok := s.Deployment.Remote.(FileDispatcher)
	if !ok {
		return model.FileResult{}, ErrDeploymentUnavailable
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	return files.Files(ctx, sparkID, model.FileRequest{ServerID: namespace, Operation: operation, Path: name, Offset: offset})
}
