package repository

import (
	"context"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/integrations"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

const (
	messageHistoryAge   = 30 * 24 * time.Hour
	messageHistoryLimit = 10000
	messageReceiptAge   = 90 * 24 * time.Hour
	messageReceiptLimit = 100000
)

// PruneMessageHistory bounds terminal history and retains compact event receipts
// for deduplication. Pending, ambiguous, blocked, and card work is never pruned.
func (r *IntegrationRepository) PruneMessageHistory(ctx context.Context, now time.Time) error {
	var bindings []string

	states := []model.DeliveryState{model.DeliveryDelivered, model.DeliveryFailed, integrations.MessageReceipt}
	if err := r.db.WithContext(ctx).Model(&database.IntegrationMessage{}).
		Where("category <> ? AND state IN ?", integrations.CardCategory, states).
		Distinct().Pluck("binding_id", &bindings).Error; err != nil {
		return err
	}

	for _, binding := range bindings {
		if err := database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
			for _, policy := range []struct {
				states []model.DeliveryState
				age    time.Duration
				limit  int
				purge  bool
			}{
				{states[:2], messageHistoryAge, messageHistoryLimit, false},
				{states[2:], messageReceiptAge, messageReceiptLimit, true},
			} {
				query := tx.Model(&database.IntegrationMessage{}).
					Where("binding_id = ? AND category <> ? AND state IN ?", binding, integrations.CardCategory, policy.states)

				var cutoff database.IntegrationMessage
				if err := query.Session(&gorm.Session{}).Select("id", "created_at").
					Order("created_at DESC, id DESC").Offset(policy.limit).Limit(1).Find(&cutoff).Error; err != nil {
					return err
				}

				if cutoff.ID != 0 {
					query = query.Where("created_at < ? OR created_at < ? OR (created_at = ? AND id <= ?)", now.Add(-policy.age).Unix(), cutoff.CreatedAt, cutoff.CreatedAt, cutoff.ID)
				} else {
					query = query.Where("created_at < ?", now.Add(-policy.age).Unix())
				}

				if policy.purge {
					if err := query.Delete(&database.IntegrationMessage{}).Error; err != nil {
						return err
					}
				} else if err := query.Updates(map[string]any{
					"state": integrations.MessageReceipt, "payload": "{}", "error": "",
					"external_message_id": "", "delivered_at": 0, "attempts": 0, "next_attempt_at": 0,
				}).Error; err != nil {
					return err
				}
			}

			return nil
		}); err != nil {
			return err
		}
	}

	return nil
}
