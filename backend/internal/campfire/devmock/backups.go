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
	"campfire/internal/shared/backuparchive"
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
	record.Options.StopServer = false

	files := maps.Clone(record.Files)
	if record.Options.Mode != "" {
		metadata := backuparchive.Manifest{Format: 1, BackupOptions: record.Options, Missing: record.Missing}
		if record.Options.Definition != nil {
			metadata.Format = backuparchive.Format
		}

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
			if record.State == model.RuntimeStateNotInstalled || record.State == model.RuntimeStateDeleted {
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
	if request.Action == model.ServerActionRestore && record.State != model.RuntimeStateStopped && record.State != model.RuntimeStateNotInstalled {
		return fileError(model.FileBusy, "Stop the server before restoring backups.")
	}

	if request.Action == model.ServerActionBackup {
		if record.State != model.RuntimeStateStopped && record.State != model.RuntimeStateRunning {
			return fileError(model.FileBusy, "Wait until the server is running or stopped before creating a backup.")
		}

		if request.BackupOptions.StopServer && record.State == model.RuntimeStateRunning {
			record.State = model.RuntimeStateStopped

			record.Logs += timestampLog("[Mock Spark] Server stopped for backup.\n")
			defer func() {
				record.State = model.RuntimeStateRunning
				record.ActiveRevision = model.ContainerRevision(record.Deployment)
				record.StartedAt, record.StoppedAt = r.now().Unix(), 0
				record.Logs += timestampLog("[Mock Spark] Server started after backup.\n")
			}()
		}

		options := request.BackupOptions
		options.Mode = options.Mode.Normalized()

		if err := options.Validate(); err != nil {
			return err
		}

		archiveID := request.OperationID
		if request.ArchiveID != "" {
			archiveID = request.ArchiveID
		}

		name := options.Mode.ArchiveName(archiveID)
		if request.OperationID == "" || !model.BackupName(name) {
			return fileError(model.FileInvalid, "Invalid backup operation ID.")
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
		return fileError(model.FileUnsupportedCapability, "Off-host restores are not simulated in mock mode.")
	}

	var backup backupRecord

	if err := r.db.WithContext(ctx).Where("server_id = ? AND name = ?", record.ServerID, request.Content).Take(&backup).Error; err != nil {
		return fileError(model.FileNotFound, "Backup not found.")
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
		return fileError(model.FileLimitExceeded, "Restored files would exceed the game-data allocation.")
	}

	record.Files = restored.Files
	record.State = model.RuntimeStateStopped

	return nil
}

func (r *Runtime) backupFiles(ctx context.Context, record runtimeRecord, request model.FileRequest) (model.FileResult, error) {
	result := model.FileResult{ServerID: record.ServerID, Path: request.Path}
	if request.Operation.IsBackupUpload() {
		return r.uploadBackup(ctx, record, request)
	}

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

			result.Entries = append(result.Entries, model.FileEntry{Portable: backup.Options.Definition != nil, BackupMode: backup.Options.Mode.Normalized(), Name: backup.Name, Kind: model.FileKindFile, Size: int64(len(data)), ModifiedAt: backup.CreatedAt})
		}

		return result, nil
	}

	if !model.BackupName(request.Path) || request.Offset < 0 {
		return result, fileError(model.FileInvalid, "Invalid backup request.")
	}

	if request.Operation == model.FileOperationBackupDelete {
		return result, r.db.WithContext(ctx).Where("server_id = ? AND name = ?", record.ServerID, request.Path).Delete(&backupRecord{}).Error
	}

	if request.Operation != model.FileOperationBackupRead {
		return result, fileError(model.FileInvalid, "Invalid backup operation.")
	}

	var backup backupRecord

	if err := r.db.WithContext(ctx).Where("server_id = ? AND name = ?", record.ServerID, request.Path).Take(&backup).Error; err != nil {
		return result, fileError(model.FileNotFound, "Backup not found.")
	}

	data, err := backupArchive(backup)
	if err != nil {
		return result, err
	}

	result.Size = int64(len(data))
	if request.Offset > result.Size {
		return result, fileError(model.FileInvalid, "Invalid backup offset.")
	}

	result.Data = data[request.Offset:min(result.Size, request.Offset+model.FileLimit)]

	return result, nil
}
