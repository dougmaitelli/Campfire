//go:build !production

package devmock

import (
	"context"
	"errors"
	"maps"
	"strings"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

const backupModesSeedMarkerID = 7

func seedBackupModes(ctx context.Context, db *gorm.DB) error {
	return database.Transaction(db.WithContext(ctx), func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&seedMarker{}).Where("id = ?", backupModesSeedMarkerID).Count(&count).Error; err != nil || count > 0 {
			return err
		}

		ember, err := repository.NewEmberRepository(tx).Get(ctx, "mock-current")
		if err != nil {
			return err
		}

		servers := repository.NewServerRepository(tx)

		server, err := servers.Get(ctx, "creative")
		if err != nil {
			return err
		}

		server.EmberID, server.EmberHash, server.StartupCommand = ember.ID, model.EmberHash(ember), "Standard"

		if err := servers.Save(ctx, server, false); err != nil {
			return err
		}

		deployment, err := model.NewDeployment(server.ID, server.ServerConfiguration, ember)
		if err != nil {
			return err
		}

		if err := tx.Model(&runtimeRecord{}).Where("server_id = ?", server.ID).Select("Deployment").Updates(&runtimeRecord{Deployment: deployment}).Error; err != nil {
			return err
		}

		for _, item := range []struct {
			name string
			mode model.BackupMode
		}{
			{"mock-before-update.tar.gz", model.BackupModeFull},
			{"mock-nightly.tar.gz", model.BackupModeSavedata},
		} {
			var backup backupRecord
			if err := tx.Where("server_id = ? AND name = ?", server.ID, item.name).Take(&backup).Error; errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			} else if err != nil {
				return err
			}

			backup.Name = item.mode.ArchiveName(strings.TrimSuffix(item.name, ".tar.gz"))
			backup.Options = model.BackupOptions{Mode: item.mode, EmberRevision: server.EmberHash}

			if item.mode == model.BackupModeSavedata {
				backup.Options.Paths = ember.SavedataPaths
				maps.DeleteFunc(backup.Files, func(name, _ string) bool { return !backup.Options.Includes(strings.TrimSuffix(name, "/")) })
			}

			if err := tx.Where("server_id = ? AND name = ?", server.ID, item.name).Delete(&backupRecord{}).Error; err != nil {
				return err
			}

			if err := tx.Create(&backup).Error; err != nil {
				return err
			}

			archive, err := backupArchive(backup)
			if err != nil {
				return err
			}

			if err := tx.Model(&database.Backup{}).Where("server_id = ? AND name = ?", server.ID, item.name).Updates(map[string]any{"name": backup.Name, "mode": item.mode, "size": len(archive), "key": server.ID + "/mock/" + backup.Name}).Error; err != nil {
				return err
			}
		}

		if err := repository.NewBackupRepository(tx).SavePolicy(ctx, server.ID, model.BackupPolicy{Storage: "local", LocalKeepLast: 3, LocalMaxAgeDays: 30, Savedata: model.BackupRetention{LocalKeepLast: 12, LocalMaxAgeDays: 7}}); err != nil {
			return err
		}

		return tx.Create(&seedMarker{ID: backupModesSeedMarkerID}).Error
	})
}
