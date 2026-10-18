package repository

import (
	"context"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

// EmberUpdateRepository stores check results on the current Ember row.
type EmberUpdateRepository struct{ db *gorm.DB }

// NewEmberUpdateRepository creates a check-result repository.
func NewEmberUpdateRepository(db *gorm.DB) *EmberUpdateRepository {
	return &EmberUpdateRepository{db: db}
}

// List returns persisted update checks.
func (r *EmberUpdateRepository) List(ctx context.Context) ([]model.EmberUpdate, error) {
	var rows []database.Ember
	if err := r.db.WithContext(ctx).Where("update_checked_at > 0").Find(&rows).Error; err != nil {
		return nil, err
	}

	items := make([]model.EmberUpdate, 0, len(rows))
	for _, row := range rows {
		items = append(items, model.EmberUpdate{EmberID: row.ID, CheckedAt: row.UpdateCheckedAt, Available: row.UpdateAvailable, Error: row.UpdateError})
	}

	return items, nil
}

// Save replaces a revision's latest check result.
func (r *EmberUpdateRepository) Save(ctx context.Context, item model.EmberUpdate) error {
	result := r.db.WithContext(ctx).Model(&database.Ember{}).Where("id = ?", item.EmberID).Updates(map[string]any{"update_checked_at": item.CheckedAt, "update_available": item.Available, "update_error": item.Error})
	if result.Error == nil && result.RowsAffected == 0 {
		return ErrEmberNotFound
	}

	return result.Error
}
