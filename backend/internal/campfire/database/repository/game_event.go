package repository

import (
	"context"
	"errors"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrGameEventPlacement rejects ingestion from a deleted or reassigned server.
var ErrGameEventPlacement = errors.New("game event source does not own server")

// GameEventQuery uses a receipt-order cursor independent of game clocks.
type GameEventQuery struct {
	Before int64
	From   int64
	To     int64
	Type   string
	Limit  int
}

// GameEventPage contains a bounded page and its next receipt-order cursor.
type GameEventPage struct {
	Items      []database.GameEvent `json:"items"`
	NextBefore int64                `json:"next_before,omitempty"`
}

// GameEventRepository owns durable deduplication and bounded retained history.
type GameEventRepository struct{ db *gorm.DB }

// NewGameEventRepository uses the panel's shared transactional database.
func NewGameEventRepository(db *gorm.DB) *GameEventRepository { return &GameEventRepository{db: db} }

// Append commits the event, source check, and retention changes together.
func (r *GameEventRepository) Append(ctx context.Context, sparkID, serverID string, event model.GameEvent, now time.Time, retention time.Duration, maximum int) error {
	return database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&database.Server{}).Where("id = ? AND spark_id = ?", serverID, sparkID).Count(&count).Error; err != nil {
			return err
		}

		if count == 0 {
			return ErrGameEventPlacement
		}

		row := database.GameEvent{ServerID: serverID, SparkID: sparkID, EventID: event.ID, Type: event.Type, Title: event.Title, OccurredAt: event.OccurredAt.UTC(), ReceivedAt: now.Unix(), Data: event.Data}
		if row.Data == nil {
			row.Data = map[string]any{}
		}

		if err := tx.Omit("Server").Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "server_id"}, {Name: "event_id"}}, DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}

		return pruneGameServer(tx, serverID, now.Add(-retention).Unix(), maximum)
	})
}

func pruneGameServer(tx *gorm.DB, serverID string, before int64, maximum int) error {
	if err := tx.Where("server_id = ? AND received_at < ?", serverID, before).Delete(&database.GameEvent{}).Error; err != nil {
		return err
	}

	newest := tx.Model(&database.GameEvent{}).Select("id").Where("server_id = ?", serverID).Order("id DESC").Limit(maximum)

	return tx.Where("server_id = ? AND id NOT IN (?)", serverID, newest).Delete(&database.GameEvent{}).Error
}

// List returns receipt-ordered events; optional time filters use occurrence time.
func (r *GameEventRepository) List(ctx context.Context, serverID string, input GameEventQuery) (GameEventPage, error) {
	page := GameEventPage{Items: []database.GameEvent{}}
	if input.Limit < 1 || input.Limit > 100 {
		return page, errors.New("invalid game event page limit")
	}

	query := r.db.WithContext(ctx).Where("server_id = ?", serverID)
	if input.Before > 0 {
		query = query.Where("id < ?", input.Before)
	}

	if input.Type != "" {
		query = query.Where("type = ?", input.Type)
	}

	if input.From > 0 {
		query = query.Where("occurred_at >= ?", time.Unix(input.From, 0).UTC())
	}

	if input.To > 0 {
		query = query.Where("occurred_at < ?", time.Unix(input.To, 0).UTC())
	}

	if err := query.Order("id DESC").Limit(input.Limit + 1).Find(&page.Items).Error; err != nil {
		return page, err
	}

	if len(page.Items) > input.Limit {
		page.Items = page.Items[:input.Limit]
		page.NextBefore = page.Items[len(page.Items)-1].ID
	}

	return page, nil
}

// Prune also enforces reduced limits and expiration when no new events arrive.
func (r *GameEventRepository) Prune(ctx context.Context, before int64, maximum int) error {
	var ids []string
	if err := r.db.WithContext(ctx).Model(&database.GameEvent{}).Distinct().Pluck("server_id", &ids).Error; err != nil {
		return err
	}

	for _, id := range ids {
		if err := database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error { return pruneGameServer(tx, id, before, maximum) }); err != nil {
			return err
		}
	}

	return nil
}
