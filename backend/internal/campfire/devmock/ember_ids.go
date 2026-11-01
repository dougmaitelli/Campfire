//go:build !production

package devmock

import (
	"context"
	"errors"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/devfixture"

	"gorm.io/gorm"
)

func upgradeMockEmberIDs(ctx context.Context, db *gorm.DB) error {
	return database.Transaction(db.WithContext(ctx), func(tx *gorm.DB) error {
		for _, ids := range [][2]string{
			{"mock-minecraft", devfixture.MinecraftEmberID},
			{"mock-current", devfixture.CurrentMinecraftEmberID},
		} {
			if err := renameMockEmber(tx, ids[0], ids[1]); err != nil {
				return err
			}
		}

		return nil
	})
}

func renameMockEmber(tx *gorm.DB, previous, current string) error {
	var ember database.Ember
	if err := tx.Take(&ember, "id = ?", previous).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	} else if err != nil {
		return err
	}

	// Create the replacement before moving references so foreign keys stay valid.
	ember.ID, ember.Definition.ID = current, current
	if err := tx.Create(&ember).Error; err != nil {
		return err
	}

	if err := tx.Model(&database.Server{}).Where("ember_id = ?", previous).Update("ember_id", current).Error; err != nil {
		return err
	}

	var records []runtimeRecord
	if err := tx.Find(&records).Error; err != nil {
		return err
	}

	for _, record := range records {
		if record.Deployment.Configuration.EmberID == previous {
			record.Deployment.Configuration.EmberID = current
			if err := tx.Model(&record).Select("Deployment").Updates(&record).Error; err != nil {
				return err
			}
		}
	}

	var staged []transferDeployment
	if err := tx.Find(&staged).Error; err != nil {
		return err
	}

	for _, record := range staged {
		if record.Deployment.Configuration.EmberID == previous {
			record.Deployment.Configuration.EmberID = current
			if err := tx.Model(&record).Select("Deployment").Updates(&record).Error; err != nil {
				return err
			}
		}
	}

	var transfers []database.ServerTransfer
	if err := tx.Find(&transfers).Error; err != nil {
		return err
	}

	for _, transfer := range transfers {
		if transfer.SourceConfiguration.EmberID != previous && transfer.TargetConfiguration.EmberID != previous {
			continue
		}

		if transfer.SourceConfiguration.EmberID == previous {
			transfer.SourceConfiguration.EmberID = current
		}

		if transfer.TargetConfiguration.EmberID == previous {
			transfer.TargetConfiguration.EmberID = current
		}

		if err := tx.Model(&transfer).Select("SourceConfiguration", "TargetConfiguration").Updates(&transfer).Error; err != nil {
			return err
		}
	}

	return tx.Delete(&database.Ember{}, "id = ?", previous).Error
}
