//go:build !production

package devmock

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"strings"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/devfixture"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

type backupRecord struct {
	Missing   []string            `gorm:"serializer:json;type:text"`
	Options   model.BackupOptions `gorm:"serializer:json;type:text"`
	ServerID  string              `gorm:"primaryKey"`
	Name      string              `gorm:"primaryKey"`
	Files     map[string]string   `gorm:"serializer:json"`
	CreatedAt time.Time
}

func (backupRecord) TableName() string { return "mock_backups" }

func backupArchive(record backupRecord) ([]byte, error) {
	files := maps.Clone(record.Files)
	if record.Options.Mode != "" {
		metadata := struct {
			Format int `json:"format"`
			model.BackupOptions
			Missing []string `json:"missing,omitempty"`
		}{1, record.Options, record.Missing}

		data, err := json.Marshal(metadata)
		if err != nil {
			return nil, err
		}

		files[model.BackupMetadataName] = string(data)
	}

	return devfixture.Archive(files, record.CreatedAt)
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

			for _, entry := range devfixture.Backups(time.Now().UTC()) {
				name := entry.Name

				backup := backupRecord{ServerID: record.ServerID, Name: name, Files: maps.Clone(record.Files), CreatedAt: entry.ModifiedAt}
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

func upgradeBackupSizes(ctx context.Context, db *gorm.DB) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var marker seedMarker

		if err := tx.Take(&marker, "id = ?", 3).Error; err == nil {
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		var backups []backupRecord

		if err := tx.Where("name IN ?", []string{"mock-nightly.tar.gz", "mock-before-update.tar.gz"}).Find(&backups).Error; err != nil {
			return err
		}

		for _, backup := range backups {
			backup.Files = devfixture.BackupFiles(backup.Files, backup.ServerID, backup.Name)

			if err := tx.Save(&backup).Error; err != nil {
				return err
			}

			archive, err := backupArchive(backup)
			if err != nil {
				return err
			}

			if err := tx.Model(&database.Backup{}).Where("server_id = ? AND name = ?", backup.ServerID, backup.Name).Update("size", len(archive)).Error; err != nil {
				return err
			}
		}

		return tx.Create(&seedMarker{ID: 3}).Error
	})
}

func (r *Runtime) backupAction(ctx context.Context, record *runtimeRecord, request model.RuntimeRequest) error {
	if record.State != "stopped" {
		return fileError("file_busy", "Stop the server before creating or restoring backups.")
	}

	if request.Action == "backup" {
		options := request.BackupOptions
		options.Mode = options.Mode.Normalized()

		if err := options.Validate(); err != nil {
			return err
		}

		name := options.Mode.ArchiveName(request.OperationID)
		if request.OperationID == "" || !model.BackupName(name) {
			return fileError("invalid_file", "Invalid backup operation ID.")
		}

		backup := backupRecord{ServerID: record.ServerID, Name: name, Files: maps.Clone(record.Files), CreatedAt: time.Now().UTC()}
		backup.Options = options

		if options.Mode == model.BackupModeSavedata {
			for _, item := range options.Paths {
				found := false
				for name := range record.Files {
					found = found || name == item.Path || strings.HasPrefix(name, item.Path+"/")
				}

				if item.Required && !found {
					return errors.New("required save-data path is missing")
				}

				if !found {
					backup.Missing = append(backup.Missing, item.Path)
				}
			}

			if len(backup.Missing) == len(options.Paths) {
				return errors.New("no declared save-data paths exist")
			}

			maps.DeleteFunc(backup.Files, func(name, _ string) bool { return !options.Includes(strings.TrimSuffix(name, "/")) })
		}

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

	if backup.Options.Mode == model.BackupModeSavedata {
		for name, content := range record.Files {
			if !backup.Options.Includes(strings.TrimSuffix(name, "/")) {
				restored.Files[name] = content
			}
		}
	}

	if quotaEnforced(restored) && diskUsage(restored) > record.Deployment.Configuration.DiskMB*(1<<20) {
		return fileError("file_limit", "Restored files would exceed the game-data allocation.")
	}

	record.Files = restored.Files

	return nil
}

func (r *Runtime) backupFiles(ctx context.Context, record runtimeRecord, request model.FileRequest) (model.FileResult, error) {
	result := model.FileResult{ServerID: record.ServerID, Path: request.Path}
	if request.Operation == model.FileOperationBackupList {
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

			result.Entries = append(result.Entries, model.FileEntry{BackupMode: backup.Options.Mode.Normalized(), Name: backup.Name, Kind: "file", Size: int64(len(data)), ModifiedAt: backup.CreatedAt})
		}

		return result, nil
	}

	if !model.BackupName(request.Path) || request.Offset < 0 {
		return result, fileError("invalid_file", "Invalid backup request.")
	}

	if request.Operation == model.FileOperationBackupDelete {
		return result, r.db.WithContext(ctx).Where("server_id = ? AND name = ?", record.ServerID, request.Path).Delete(&backupRecord{}).Error
	}

	if request.Operation != model.FileOperationBackupRead {
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
