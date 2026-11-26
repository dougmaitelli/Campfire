//go:build !production

package devmock

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"errors"
	"io"
	"strings"
	"time"

	"campfire/internal/shared/backuparchive"
	"campfire/internal/shared/model"
)

const mockBackupUploadLimit = 64 << 20

type mockBackupUpload struct {
	server, path string
	size         int64
	data         bytes.Buffer
	expires      time.Time
}

func (r *Runtime) uploadBackup(ctx context.Context, record runtimeRecord, q model.FileRequest) (model.FileResult, error) {
	result := model.FileResult{ServerID: record.ServerID, Path: q.Path, UploadID: q.UploadID}
	if !model.BackupName(q.Path) {
		return result, fileError(model.FileInvalid, "Invalid backup name.")
	}

	if r.backupUploads == nil {
		r.backupUploads = make(map[string]*mockBackupUpload)
	}

	for id, upload := range r.backupUploads {
		if time.Now().After(upload.expires) {
			delete(r.backupUploads, id)
		}
	}

	if q.Operation == model.FileOperationBackupUploadBegin {
		if q.Size < 1 || q.Size > min(q.UploadLimit, mockBackupUploadLimit) || len(r.backupUploads) >= 8 {
			return result, fileError(model.FileLimitExceeded, "Mock backup uploads are limited to 64 MiB and eight sessions.")
		}

		result.UploadID = rand.Text()
		r.backupUploads[result.UploadID] = &mockBackupUpload{server: record.ServerID, path: q.Path, size: q.Size, expires: time.Now().Add(time.Hour)}

		return result, nil
	}

	upload := r.backupUploads[q.UploadID]
	if upload == nil || upload.server != record.ServerID || upload.path != q.Path {
		return result, fileError(model.FileNotFound, "Backup upload unavailable.")
	}

	switch q.Operation {
	case model.FileOperationBackupUploadCancel:
		delete(r.backupUploads, q.UploadID)
		return result, nil
	case model.FileOperationBackupUploadChunk:
		if q.Offset != int64(upload.data.Len()) || len(q.Data) > model.FileLimit || int64(len(q.Data)) > upload.size-q.Offset {
			return result, fileError(model.FileInvalid, "Invalid backup upload chunk.")
		}

		_, _ = upload.data.Write(q.Data)
		result.Size = int64(upload.data.Len())

		return result, nil
	case model.FileOperationBackupUploadCommit:
		if int64(upload.data.Len()) != upload.size {
			return result, fileError(model.FileInvalid, "Incomplete backup upload.")
		}

		manifest, err := backuparchive.Inspect(ctx, bytes.NewReader(upload.data.Bytes()))
		if err != nil {
			return result, err
		}

		compressed, err := gzip.NewReader(bytes.NewReader(upload.data.Bytes()))
		if err != nil {
			return result, err
		}

		defer func() { _ = compressed.Close() }()

		archive := tar.NewReader(compressed)
		files := make(map[string]string)

		var total int64

		for {
			header, err := archive.Next()
			if errors.Is(err, io.EOF) {
				break
			}

			if err != nil {
				return result, err
			}

			if header.Name == model.BackupMetadataName {
				continue
			}

			if header.Size > mockBackupUploadLimit-total {
				return result, fileError(model.FileLimitExceeded, "Mock backup entry is too large.")
			}

			total += header.Size
			if header.Typeflag == tar.TypeDir {
				files[strings.TrimSuffix(header.Name, "/")+"/"] = ""
				continue
			}

			data, err := io.ReadAll(archive)
			if err != nil {
				return result, err
			}

			files[header.Name] = string(data)
		}

		backup := backupRecord{ServerID: record.ServerID, Name: q.Path, Options: manifest.BackupOptions, Missing: manifest.Missing, Files: files, CreatedAt: time.Now().UTC()}
		if err := r.db.WithContext(ctx).Create(&backup).Error; err != nil {
			return result, err
		}

		delete(r.backupUploads, q.UploadID)

		return result, nil
	default:
		return result, fileError(model.FileInvalid, "Invalid backup upload operation.")
	}
}
