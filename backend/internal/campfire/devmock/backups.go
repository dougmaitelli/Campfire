//go:build !production

package devmock

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"maps"
	"sort"
	"strings"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

type backupRecord struct {
	ServerID  string            `gorm:"primaryKey"`
	Name      string            `gorm:"primaryKey"`
	Files     map[string]string `gorm:"serializer:json"`
	CreatedAt time.Time
}

func (backupRecord) TableName() string { return "mock_backups" }

func backupArchive(record backupRecord) ([]byte, error) {
	var buffer bytes.Buffer

	zipped := gzip.NewWriter(&buffer)
	archive := tar.NewWriter(zipped)
	names := make([]string, 0, len(record.Files))

	for name := range record.Files {
		names = append(names, name)
	}

	sort.Strings(names)

	for _, name := range names {
		content := record.Files[name]
		header := &tar.Header{Name: name, Mode: 0600, Size: int64(len(content)), ModTime: record.CreatedAt}

		if strings.HasSuffix(name, "/") {
			header.Typeflag, header.Mode, header.Size = tar.TypeDir, 0700, 0
		}

		if err := archive.WriteHeader(header); err != nil {
			return nil, err
		}

		if _, err := archive.Write([]byte(content)); err != nil {
			return nil, err
		}
	}

	if err := archive.Close(); err != nil {
		return nil, err
	}

	if err := zipped.Close(); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

func seedBackupFixtures(ctx context.Context, db *gorm.DB) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var marker seedMarker

		if err := tx.Take(&marker, "id = ?", 2).Error; err == nil {
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		var records []runtimeRecord

		if err := tx.Find(&records).Error; err != nil {
			return err
		}

		for _, record := range records {
			if record.State == "not_installed" || record.State == "deleted" {
				continue
			}

			for index, name := range []string{"mock-nightly.tar.gz", "mock-before-update.tar.gz"} {
				backup := backupRecord{ServerID: record.ServerID, Name: name, Files: maps.Clone(record.Files), CreatedAt: time.Now().UTC().Add(-time.Duration(index+1) * 24 * time.Hour)}
				if err := tx.Create(&backup).Error; err != nil {
					return err
				}

				data, err := backupArchive(backup)
				if err != nil {
					return err
				}

				if err := repository.NewBackupRepository(tx).Discover(ctx, database.Backup{ServerID: record.ServerID, Name: name, Size: int64(len(data)), CreatedAt: backup.CreatedAt.UnixNano(), Local: true, Key: record.ServerID + "/mock/" + name}); err != nil {
					return err
				}
			}
		}

		return tx.Create(&seedMarker{ID: 2}).Error
	})
}

func (r *Runtime) backupAction(ctx context.Context, record *runtimeRecord, request model.RuntimeRequest) error {
	if record.State != "stopped" {
		return fileError("file_busy", "Stop the server before creating or restoring backups.")
	}

	if request.Action == "backup" {
		name := request.OperationID + ".tar.gz"
		if !model.BackupName(name) {
			return fileError("invalid_file", "Invalid backup operation ID.")
		}

		backup := backupRecord{ServerID: record.ServerID, Name: name, Files: maps.Clone(record.Files), CreatedAt: time.Now().UTC()}

		return r.db.WithContext(ctx).Where("server_id = ? AND name = ?", record.ServerID, name).FirstOrCreate(&backup).Error
	}

	if request.BackupSource != nil {
		return fileError("unsupported_capability", "Off-host restores are not simulated in mock mode.")
	}

	var backup backupRecord

	if err := r.db.WithContext(ctx).Where("server_id = ? AND name = ?", record.ServerID, request.Content).Take(&backup).Error; err != nil {
		return fileError("file_not_found", "Backup not found.")
	}

	restored := *record
	restored.Files = maps.Clone(backup.Files)

	if quotaEnforced(restored) && diskUsage(restored) > record.Deployment.Configuration.DiskMB*(1<<20) {
		return fileError("file_limit", "Restored files would exceed the game-data allocation.")
	}

	record.Files = restored.Files

	return nil
}

func (r *Runtime) backupFiles(ctx context.Context, record runtimeRecord, request model.FileRequest) (model.FileResult, error) {
	result := model.FileResult{ServerID: record.ServerID, Path: request.Path}
	if request.Operation == "backup.list" {
		var backups []backupRecord

		if err := r.db.WithContext(ctx).Where("server_id = ?", record.ServerID).Order("created_at DESC").Find(&backups).Error; err != nil {
			return result, err
		}

		result.Entries = []model.FileEntry{}

		for _, backup := range backups {
			data, err := backupArchive(backup)
			if err != nil {
				return result, err
			}

			result.Entries = append(result.Entries, model.FileEntry{Name: backup.Name, Kind: "file", Size: int64(len(data)), ModifiedAt: backup.CreatedAt})
		}

		return result, nil
	}

	if !model.BackupName(request.Path) || request.Offset < 0 {
		return result, fileError("invalid_file", "Invalid backup request.")
	}

	if request.Operation == "backup.delete" {
		return result, r.db.WithContext(ctx).Where("server_id = ? AND name = ?", record.ServerID, request.Path).Delete(&backupRecord{}).Error
	}

	if request.Operation != "backup.read" {
		return result, fileError("invalid_file", "Invalid backup operation.")
	}

	var backup backupRecord

	if err := r.db.WithContext(ctx).Where("server_id = ? AND name = ?", record.ServerID, request.Path).Take(&backup).Error; err != nil {
		return result, fileError("file_not_found", "Backup not found.")
	}

	data, err := backupArchive(backup)
	if err != nil {
		return result, err
	}

	result.Size = int64(len(data))
	if request.Offset > result.Size {
		return result, fileError("invalid_file", "Invalid backup offset.")
	}

	result.Data = data[request.Offset:min(result.Size, request.Offset+model.FileLimit)]

	return result, nil
}
