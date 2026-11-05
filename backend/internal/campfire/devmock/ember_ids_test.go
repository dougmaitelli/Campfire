//go:build !production

package devmock

import (
	"campfire/internal/shared/model"
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

	var jobs []database.ServerJob
	if err := db.ORM().Where("source_server_id = ?", original.ServerID).Find(&jobs).Error; err != nil {
		t.Fatal(err)
	}

	if len(jobs) == 0 {
		t.Fatal("missing job history")
	}

	for _, job := range jobs {
		if job.Type != model.JobTransfer || job.TargetServerID != original.ServerID {
			t.Fatal("identity upgrade changed job history", job)
		}
	}

	var count int64
	if err := db.ORM().Model(&database.Ember{}).Count(&count).Error; err != nil || count != 2 {
		t.Fatal("identity upgrade duplicated Embers", count, err)
	}
}
