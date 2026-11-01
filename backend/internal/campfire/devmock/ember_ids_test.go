//go:build !production

package devmock

import (
	"reflect"
	"testing"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/devfixture"

	"gorm.io/gorm"
)

func TestMockEmberIdentityUpgradePreservesSavedData(t *testing.T) {
	_, db, auth, _ := fixture(t)
	ctx := t.Context()

	var original runtimeRecord
	if err := db.ORM().Take(&original, "server_id = ?", "creative").Error; err != nil {
		t.Fatal(err)
	}

	if err := database.Transaction(db.ORM(), func(tx *gorm.DB) error {
		if err := tx.Exec("PRAGMA defer_foreign_keys = ON").Error; err != nil {
			return err
		}

		var ember database.Ember
		if err := tx.Take(&ember, "id = ?", devfixture.CurrentMinecraftEmberID).Error; err != nil {
			return err
		}

		ember.ID, ember.Definition.ID = "mock-current", "mock-current"
		if err := tx.Model(&database.Ember{}).Where("id = ?", devfixture.CurrentMinecraftEmberID).Select("ID", "Definition").Updates(&ember).Error; err != nil {
			return err
		}

		if err := tx.Model(&database.Server{}).Where("ember_id = ?", devfixture.CurrentMinecraftEmberID).Update("ember_id", ember.ID).Error; err != nil {
			return err
		}

		record := original

		record.Deployment.Configuration.EmberID = ember.ID
		if err := tx.Model(&record).Select("Deployment").Updates(&record).Error; err != nil {
			return err
		}

		if err := tx.Create(&transferDeployment{ServerID: record.ServerID, SparkID: "mock-america", Deployment: record.Deployment}).Error; err != nil {
			return err
		}

		var transfers []database.ServerTransfer
		if err := tx.Where("server_id = ?", record.ServerID).Find(&transfers).Error; err != nil {
			return err
		}

		for _, transfer := range transfers {
			transfer.SourceConfiguration.EmberID, transfer.TargetConfiguration.EmberID = ember.ID, ember.ID
			if err := tx.Model(&transfer).Select("SourceConfiguration", "TargetConfiguration").Updates(&transfer).Error; err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		t.Fatal(err)
	}

	for range 2 {
		if _, err := New(ctx, db.ORM(), auth); err != nil {
			t.Fatal(err)
		}
	}

	server, err := repository.NewServerRepository(db.ORM()).Get(ctx, original.ServerID)
	if err != nil || server.EmberID != devfixture.CurrentMinecraftEmberID || server.EmberUpdateAvailable {
		t.Fatal("identity upgrade changed the applied Ember", server, err)
	}

	var restored runtimeRecord
	if err := db.ORM().Take(&restored, "server_id = ?", original.ServerID).Error; err != nil || !reflect.DeepEqual(restored, original) {
		t.Fatal("identity upgrade changed runtime data", restored, err)
	}

	var staged transferDeployment
	if err := db.ORM().Take(&staged, "server_id = ?", original.ServerID).Error; err != nil || !reflect.DeepEqual(staged.Deployment, original.Deployment) {
		t.Fatal("staged transfer kept the old Ember identity", staged, err)
	}

	var transfers []database.ServerTransfer
	if err := db.ORM().Where("server_id = ?", original.ServerID).Find(&transfers).Error; err != nil {
		t.Fatal(err)
	}

	if len(transfers) == 0 {
		t.Fatal("missing transfer history")
	}

	for _, transfer := range transfers {
		if transfer.SourceConfiguration.EmberID != server.EmberID || transfer.TargetConfiguration.EmberID != server.EmberID {
			t.Fatal("transfer history changed during identity upgrade", transfer)
		}
	}

	var count int64
	if err := db.ORM().Model(&database.Ember{}).Count(&count).Error; err != nil || count != 2 {
		t.Fatal("identity upgrade duplicated Embers", count, err)
	}
}
