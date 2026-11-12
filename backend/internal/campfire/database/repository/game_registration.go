package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrGameRegistrationConflict rejects stale revisions and revision reuse with different definitions.
var ErrGameRegistrationConflict = errors.New("game registration requires a newer revision")

// GameRegistration loads persistent definitions. A missing registration keeps legacy publishers working.
func (r *GameEventRepository) GameRegistration(ctx context.Context, serverID string) (*model.GameRegistration, error) {
	var row database.GameIntegrationRegistration
	if err := r.db.WithContext(ctx).Take(&row, "server_id = ?", serverID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}

		return nil, err
	}

	return &row.Definition, nil
}

// RegisterGame atomically replaces definitions after checking current placement.
// Locking the owning server serializes concurrent registration revisions on PostgreSQL.
func (r *GameEventRepository) RegisterGame(ctx context.Context, sparkID, serverID string, definition model.GameRegistration) error {
	if err := (model.GamePublication{Version: 1, Registration: &definition}).Validate(); err != nil {
		return err
	}

	return database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		var server database.Server
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND spark_id = ? AND game_integration = ?", serverID, sparkID, true).Take(&server).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrGameEventPlacement
			}

			return err
		}

		var previous database.GameIntegrationRegistration

		err := tx.Take(&previous, "server_id = ?", serverID).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if err == nil {
			oldJSON, _ := json.Marshal(previous.Definition)

			newJSON, _ := json.Marshal(definition)
			if bytes.Equal(oldJSON, newJSON) {
				return nil
			}

			if definition.Revision <= previous.Definition.Revision {
				return ErrGameRegistrationConflict
			}
		}

		row := database.GameIntegrationRegistration{ServerID: serverID, Revision: definition.Revision, Definition: definition}
		if err := tx.Omit("Server").Save(&row).Error; err != nil {
			return err
		}
		// Registration is authoritative: removing a definition restores raw-key display.
		if err := tx.Where("server_id = ?", serverID).Delete(&database.GameEventFieldLabel{}).Error; err != nil {
			return err
		}

		rows := []database.GameEventFieldLabel{}

		for eventType, labels := range definition.EventFields {
			for key, label := range labels {
				rows = append(rows, database.GameEventFieldLabel{ServerID: serverID, EventType: eventType, FieldKey: key, Label: label})
			}
		}

		if len(rows) == 0 {
			return nil
		}

		return tx.Omit("Server").Create(&rows).Error
	})
}

func hasGameRegistration(tx *gorm.DB, serverID string) (bool, error) {
	var count int64

	err := tx.Model(&database.GameIntegrationRegistration{}).Where("server_id = ?", serverID).Count(&count).Error

	return count != 0, err
}
