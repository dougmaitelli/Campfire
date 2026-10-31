//go:build !production

package devmock

import (
	"context"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

const variationsSeedMarkerID = 8

func seedFixtureVariations(ctx context.Context, db *gorm.DB) error {
	return database.Transaction(db.WithContext(ctx), func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&seedMarker{}).Where("id = ?", variationsSeedMarkerID).Count(&count).Error; err != nil || count > 0 {
			return err
		}

		if err := applyOrdinaryFixtureEmbers(ctx, tx); err != nil {
			return err
		}

		if err := seedFixtureAccess(tx); err != nil {
			return err
		}

		now := time.Now().UTC()
		if err := tx.Omit("Server").Create(&database.Schedule{
			ID: "savedata-backup", ServerID: "creative", Name: "Save-data backup", Action: "backup", BackupMode: model.BackupModeSavedata,
			TimeZone: "UTC", NextRunAt: now.Add(6 * time.Hour).Unix(), Enabled: false, RunAsUserID: 1, CreatedAt: now.Unix(),
		}).Error; err != nil {
			return err
		}

		if err := repository.NewActivityRepository(tx).CreateRun(ctx, &database.ScheduleRun{
			ID: "failed-restart", ScheduleID: "nightly-restart", ServerID: "survival", ServerName: "Survival", UserID: 1, Username: Username,
			Action: "restart", Recurring: true, State: "failed", Message: "Restart request failed while Spark was temporarily unavailable.",
			CreatedAt: now.Add(-30 * time.Minute).Unix(), StartedAt: now.Add(-30 * time.Minute).Unix(), FinishedAt: now.Add(-29 * time.Minute).Unix(),
		}); err != nil {
			return err
		}

		return tx.Create(&seedMarker{ID: variationsSeedMarkerID}).Error
	})
}

func applyOrdinaryFixtureEmbers(ctx context.Context, tx *gorm.DB) error {
	ember, err := repository.NewEmberRepository(tx).Get(ctx, mockImportedUpdateID)
	if err != nil {
		return err
	}

	servers := repository.NewServerRepository(tx)
	for _, id := range []string{"survival", "modded", "unavailable"} {
		server, err := servers.Get(ctx, id)
		if err != nil {
			return err
		}

		if server.EmberID != ember.ID || !server.EmberUpdateAvailable {
			continue
		}

		server.EmberHash = model.EmberHash(ember)
		if err := servers.Save(ctx, server, false); err != nil {
			return err
		}

		deployment, err := model.NewDeployment(server.ID, server.ServerConfiguration, ember)
		if err != nil {
			return err
		}

		if err := tx.Model(&runtimeRecord{}).Where("server_id = ?", id).Select("Deployment").Updates(&runtimeRecord{Deployment: deployment}).Error; err != nil {
			return err
		}
	}

	return nil
}

func seedFixtureAccess(tx *gorm.DB) error {
	var owner database.User
	if err := tx.Where("username = ?", "alex").Take(&owner).Error; err != nil {
		return err
	}

	if err := tx.Model(&database.Server{}).Where("id = ?", "survival").Update("owner_user_id", owner.ID).Error; err != nil {
		return err
	}

	if err := tx.Omit("Spark", "User", "Role", "AccessRole").Create(&database.SparkAccessGrant{SparkID: "mock-europe", UserID: &owner.ID, AccessRoleID: database.CreatorRole}).Error; err != nil {
		return err
	}

	var maintainers database.Role
	if err := tx.Where("name = ? AND scope = ?", "World maintainers", "server").Take(&maintainers).Error; err != nil {
		return err
	}

	for _, item := range []struct {
		username    string
		serverID    string
		sparkID     string
		roleID      int64
		roleSubject bool
	}{
		{username: "sam", serverID: "creative", sparkID: "mock-america", roleID: database.OperatorRole},
		{username: "jordan", serverID: "modded", roleID: database.OperatorRole},
		{username: "morgan", serverID: "modded", sparkID: "mock-europe", roleID: maintainers.ID, roleSubject: true},
	} {
		var user database.User
		if err := tx.Where("username = ?", item.username).Take(&user).Error; err != nil {
			return err
		}

		userID := &user.ID

		var roleID *int64
		if item.roleSubject {
			userID, roleID = nil, &user.RoleID
		}

		if err := tx.Omit("Server", "User", "Role", "AccessRole").Create(&database.ServerAccessGrant{ServerID: item.serverID, UserID: userID, RoleID: roleID, AccessRoleID: item.roleID}).Error; err != nil {
			return err
		}

		if item.sparkID != "" {
			if err := tx.Omit("Spark", "User", "Role", "AccessRole").Create(&database.SparkAccessGrant{SparkID: item.sparkID, UserID: userID, RoleID: roleID, AccessRoleID: database.CreatorRole}).Error; err != nil {
				return err
			}
		}
	}

	return nil
}
