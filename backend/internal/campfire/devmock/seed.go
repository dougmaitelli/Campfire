//go:build !production

// Package devmock simulates Spark transports for local development without Docker.
package devmock

import (
	"context"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/devfixture"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

// Username and Password are initial credentials for the isolated development panel.
const (
	Username = "admin"
	Password = "campfire-dev-password"
)

type seedMarker struct {
	ID int `gorm:"primaryKey;autoIncrement:false"`
}

// New seeds a fresh mock database once, then restores its saved simulated state.
func New(ctx context.Context, db *gorm.DB, auth *service.AuthService) (*Runtime, error) {
	// Populate legacy rows before AutoMigrate can rebuild the SQLite table.
	if db.Migrator().HasTable(&runtimeRecord{}) {
		if !db.Migrator().HasColumn(&runtimeRecord{}, "FileFixtureVersion") {
			if err := db.WithContext(ctx).Exec("ALTER TABLE runtime_records ADD COLUMN file_fixture_version INTEGER NOT NULL DEFAULT 0").Error; err != nil {
				return nil, err
			}
		} else if err := db.WithContext(ctx).Table("runtime_records").Where("file_fixture_version IS NULL").Update("file_fixture_version", 0).Error; err != nil {
			return nil, err
		}
	}

	if err := db.WithContext(ctx).AutoMigrate(&seedMarker{}, &runtimeRecord{}, &backupRecord{}); err != nil {
		return nil, err
	}

	token, err := auth.PrepareSetup(ctx)
	if err != nil {
		return nil, err
	}

	if token != "" {
		if _, _, err := auth.Setup(ctx, token, Username, Password); err != nil {
			return nil, err
		}
	}

	err = database.Transaction(db.WithContext(ctx), func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&seedMarker{}).Count(&count).Error; err != nil || count > 0 {
			return err
		}

		if err := seed(ctx, tx); err != nil {
			return err
		}

		return tx.Create(&seedMarker{ID: 1}).Error
	})
	if err != nil {
		return nil, err
	}

	if err := database.Transaction(db.WithContext(ctx), func(tx *gorm.DB) error {
		var records []runtimeRecord
		if err := tx.Where("file_fixture_version = ?", 0).Find(&records).Error; err != nil {
			return err
		}

		for _, record := range records {
			if record.Files == nil {
				record.Files = make(map[string]string)
			}

			for name, contents := range mockFiles() {
				if _, exists := record.Files[name]; !exists {
					record.Files[name] = contents
				}
			}

			record.FileFixtureVersion = 1
			if err := tx.Save(&record).Error; err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		return nil, err
	}

	if err := seedBackupFixtures(ctx, db); err != nil {
		return nil, err
	}

	if err := upgradeBackupSizes(ctx, db); err != nil {
		return nil, err
	}

	if err := seedEmberUpdates(ctx, db); err != nil {
		return nil, err
	}

	if err := seedServerEmberUpdate(ctx, db); err != nil {
		return nil, err
	}

	if err := seedSparkClients(ctx, db); err != nil {
		return nil, err
	}

	return &Runtime{db: db}, nil
}

func seed(ctx context.Context, tx *gorm.DB) error {
	now := time.Now().UTC()

	fixtures, err := devfixture.Seed(ctx, tx, now)
	if err != nil {
		return err
	}

	ember, err := repository.NewEmberRepository(tx).Get(ctx, "mock-minecraft")
	if err != nil {
		return err
	}

	for i, fixture := range fixtures {
		record := newRecord(fixture.SparkID, model.Deployment{ServerID: fixture.ID, Configuration: fixture.ServerConfiguration, Ember: ember})
		record.State = fixture.State
		record.Logs = devfixture.ConsoleLogs

		record.StartedAt = now.Add(-time.Duration(i+1) * 24 * time.Hour).Unix()
		if fixture.State == "stopped" {
			record.StoppedAt = now.Add(-2 * time.Hour).Unix()
		}

		if err := tx.Create(&record).Error; err != nil {
			return err
		}
	}

	return nil
}
