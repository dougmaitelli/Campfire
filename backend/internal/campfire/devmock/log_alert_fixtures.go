//go:build !production

package devmock

import (
	"context"
	"errors"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/devfixture"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

const logAlertSeedMarkerID = 11

func mockLogAlerts() []model.LogAlert {
	return []model.LogAlert{{
		Title: "Minecraft EULA requires acceptance", Message: "Read Minecraft's EULA. If you agree, set eula=true in eula.txt, save it, and start the server again.",
		Severity: model.LogError, Phase: model.LogRuntime,
		Any:    []model.LogMatch{{Value: "You need to agree to the EULA", IgnoreCase: true}},
		Action: &model.LogAlertAction{Type: model.LogOpenFile, Path: "eula.txt"},
	}}
}

func withMockLogAlerts(existing []model.LogAlert) []model.LogAlert {
	for _, rule := range mockLogAlerts() {
		found := false
		for _, old := range existing {
			found = found || old.Key() == rule.Key()
		}

		if !found {
			existing = append(existing, rule)
		}
	}

	return existing
}

func seedLogAlertFixture(ctx context.Context, db *gorm.DB) error {
	return database.Transaction(db.WithContext(ctx), func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&seedMarker{}).Where("id = ?", logAlertSeedMarkerID).Count(&count).Error; err != nil || count > 0 {
			return err
		}

		servers := repository.NewServerRepository(tx)

		server, err := servers.Get(ctx, "creative")
		if errors.Is(err, repository.ErrServerNotFound) || err == nil && server.EmberID != devfixture.CurrentMinecraftEmberID {
			return tx.Create(&seedMarker{ID: logAlertSeedMarkerID}).Error
		}

		if err != nil {
			return err
		}

		embers := repository.NewEmberRepository(tx)

		ember, err := embers.Get(ctx, server.EmberID)
		if err != nil {
			return err
		}

		before, err := model.NewDeployment(server.ID, server.ServerConfiguration, ember)
		wasCurrent := err == nil && server.EmberHash == model.DeploymentHash(before)
		ember.LogAlerts = withMockLogAlerts(ember.LogAlerts)

		if err := embers.Replace(ctx, ember); err != nil {
			return err
		}

		if wasCurrent {
			before.Ember.LogAlerts = ember.LogAlerts
			if err := tx.Model(&database.Server{}).Where("id = ?", server.ID).Update("ember_hash", model.DeploymentHash(before)).Error; err != nil {
				return err
			}
		}

		var record runtimeRecord
		if err := tx.Take(&record, "server_id = ?", server.ID).Error; err != nil {
			return err
		}

		record.Deployment.Ember.LogAlerts = withMockLogAlerts(record.Deployment.Ember.LogAlerts)
		if record.LogObservation == nil || record.LogObservation.Executions[model.LogRuntime] == "" {
			record.beginLogExecution(model.LogRuntime, "mock-eula-start")
		}

		record.Logs += timestampLog("\x1b[31mYou need to agree to the EULA in order to run the server. Go to eula.txt for more info.\x1b[0m\n")
		record.matchLogAlertsInPhase(model.LogRuntime)

		if err := tx.Save(&record).Error; err != nil {
			return err
		}

		return tx.Create(&seedMarker{ID: logAlertSeedMarkerID}).Error
	})
}
