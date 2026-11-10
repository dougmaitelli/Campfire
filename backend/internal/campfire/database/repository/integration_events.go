package repository

import (
	"context"
	"encoding/json"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/integrations"

	"gorm.io/gorm"
)

// RouteEvent atomically matches every enabled provider binding and records one
// immutable outgoing message per matching binding. It performs no network I/O.
func (r *IntegrationRepository) RouteEvent(ctx context.Context, event integrations.Event, now time.Time) error {
	if err := event.Validate(); err != nil {
		return err
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		var bindings []database.ServerIntegration
		if err := tx.Joins("JOIN integrations ON integrations.id = server_integrations.integration_id").
			Where("server_integrations.server_id = ? AND server_integrations.enabled = ? AND integrations.enabled = ?", event.ServerID, true, true).
			Find(&bindings).Error; err != nil {
			return err
		}

		store := NewIntegrationRepository(tx)

		for _, binding := range bindings {
			for _, subscription := range binding.Subscriptions {
				if subscription.Matches(event) {
					if _, err := store.EnqueueMessage(ctx, binding.ID, event.ID, event.Category, payload, now); err != nil {
						return err
					}

					break
				}
			}
		}

		return nil
	})
}
