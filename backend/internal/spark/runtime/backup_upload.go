package runtime

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"campfire/internal/shared/backuparchive"
	"campfire/internal/shared/model"
	uploadstore "campfire/internal/shared/upload"
	"campfire/internal/shared/utils"
)

const backupUploadPrefix = ".campfire-backup-upload-"

// No upload sessions survive a Spark restart. Only unpublished staging files
// have this reserved prefix; completed immutable archives are left alone.
func removeAbandonedBackupUploads(directory string) {
	servers, err := os.ReadDir(directory)
	if err != nil {
		return
	}

	for _, server := range servers {
		if !server.IsDir() || !utils.IsValidResourceID(server.Name()) {
			continue
		}

		backupDirectory := filepath.Join(directory, server.Name(), "backups")

		files, err := os.ReadDir(backupDirectory)
		if err != nil {
			continue
		}

		for _, file := range files {
			if strings.HasPrefix(file.Name(), backupUploadPrefix) && file.Type().IsRegular() {
				_ = os.Remove(filepath.Join(backupDirectory, file.Name()))
			}
		}
	}
}

type backupUpload struct {
	buffer  *uploadstore.Session
	path    string
	touched time.Time
}

// Called with the runtime entry locked. Publication is immutable and follows
// complete archive validation; uploads never enter the game-data directory.
func (m *Manager) uploadBackup(ctx context.Context, e *entry, q model.FileRequest) (model.FileResult, error) {
	result := model.FileResult{ServerID: q.ServerID, Path: q.Path, UploadID: q.UploadID}
	if !model.BackupName(q.Path) || len(q.Data) > model.FileLimit {
		return result, fileFailure(model.FileInvalid, "Invalid backup upload.")
	}

	if transfer, err := m.localTransfer(q.ServerID); err != nil || (transfer != nil && !transfer.allowsOperations()) {
		return result, fileFailure(model.FileBusy, "Finish the Spark transfer before uploading backups.")
	}

	directory := filepath.Join(m.root(q.ServerID), "backups")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return result, err
	}

	if e.backupUploads == nil {
		e.backupUploads = make(map[string]*backupUpload)
	}

	for id, upload := range e.backupUploads {
		if time.Since(upload.touched) > time.Hour {
			upload.buffer.Close()

			delete(e.backupUploads, id)
		}
	}

	if q.Operation == model.FileOperationBackupUploadBegin {
		limit := model.DefaultUploadLimit
		if q.UploadLimit > 0 {
			limit = min(q.UploadLimit, model.FileTransferLimit)
		}

		if q.Size < 1 || q.Size > limit || len(e.backupUploads) >= backupUploadSessions {
			return result, fileFailure(model.FileLimitExceeded, "Backup exceeds the upload limit or too many uploads are active.")
		}

		if _, err := os.Lstat(filepath.Join(directory, q.Path)); !errors.Is(err, os.ErrNotExist) {
			return result, fileFailure(model.FileConflict, "Backup already exists.")
		}

		if err := checkBackupCapacity(directory, 1); err != nil {
			return result, err
		}

		buffer, err := uploadstore.Create(directory, backupUploadPrefix, q.Size, limit)
		if err != nil {
			return result, err
		}

		result.UploadID = rand.Text()
		e.backupUploads[result.UploadID] = &backupUpload{buffer: buffer, path: q.Path, touched: time.Now()}

		return result, nil
	}

	upload := e.backupUploads[q.UploadID]
	if upload == nil || upload.path != q.Path {
		return result, fileFailure(model.FileNotFound, "Backup upload expired or is unavailable.")
	}

	upload.touched = time.Now()
	if q.Operation == model.FileOperationBackupUploadCancel {
		upload.buffer.Close()

		delete(e.backupUploads, q.UploadID)

		return result, nil
	}

	file := upload.buffer.File

	switch q.Operation {
	case model.FileOperationBackupUploadChunk:
		if q.Offset != upload.buffer.Offset || int64(len(q.Data)) > upload.buffer.Size-upload.buffer.Offset {
			return result, fileFailure(model.FileInvalid, "Invalid backup upload offset or size.")
		}

		if err := upload.buffer.Write(q.Offset, q.Data); err != nil {
			return result, fileFailure(model.FileIO, "Could not store backup upload.")
		}

		result.Size = upload.buffer.Offset

		return result, nil
	case model.FileOperationBackupUploadCommit:
		if err := upload.buffer.Complete(); err != nil {
			return result, fileFailure(model.FileInvalid, "Backup upload is incomplete.")
		}

		if _, err := file.Seek(0, 0); err != nil {
			return result, err
		}

		// The existing staging file already owns this upload's archive slot.
		if err := checkBackupCapacity(directory, 0); err != nil {
			return result, err
		}

		manifest, err := backuparchive.Inspect(ctx, file)
		if err != nil {
			return result, fileFailure(model.FileInvalid, err.Error())
		}

		if !strings.HasPrefix(q.Path, string(manifest.Mode)+"-") {
			return result, fileFailure(model.FileInvalid, "Backup filename does not match its type.")
		}

		if err := upload.buffer.PublishNoReplace(filepath.Join(directory, q.Path)); err != nil {
			return result, fileFailure(model.FileConflict, "Backup already exists or could not be published.")
		}

		upload.buffer.Close()

		delete(e.backupUploads, q.UploadID)

		result.Size = upload.buffer.Size

		return result, syncBackupDirectory(directory)
	default:
		return result, fileFailure(model.FileInvalid, "Invalid backup upload operation.")
	}
}
