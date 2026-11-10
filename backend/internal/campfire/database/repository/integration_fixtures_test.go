package repository_test

import (
	"context"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/integrations"

	"gorm.io/gorm"
)

func seedIntegrationCardReference(ctx context.Context, db *database.Database, id string, ref integrations.MessageReference) error {
	result := db.ORM().WithContext(ctx).Model(&database.ServerIntegration{}).Where("id = ?", id).
		Updates(map[string]any{"message_destination_id": ref.DestinationID, "external_message_id": ref.ID})
	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}

	return nil
}

func readIntegrationMessage(ctx context.Context, db *database.Database, id int64) (database.IntegrationMessage, error) {
	var row database.IntegrationMessage

	err := db.ORM().WithContext(ctx).Where("id = ?", id).Take(&row).Error

	return row, err
}
