package repository

import (
	"context"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// EmberUpdateRepository stores mutable check results separately from revisions.
type EmberUpdateRepository struct{ db *gorm.DB }

// NewEmberUpdateRepository creates a check-result repository.
func NewEmberUpdateRepository(db *gorm.DB) *EmberUpdateRepository {
	return &EmberUpdateRepository{db: db}
}

// List returns persisted update checks.
func (r *EmberUpdateRepository) List(ctx context.Context) ([]model.EmberUpdate, error) {
	var rows []database.EmberUpdate
	if err := r.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, err
	}

	items := make([]model.EmberUpdate, 0, len(rows))
	for _, row := range rows {
		items = append(items, model.EmberUpdate{EmberID: row.EmberID, URL: row.URL, CheckedAt: row.CheckedAt, Available: row.Available, Error: row.Error})
	}

	return items, nil
}

// Save replaces a revision's latest check result.
func (r *EmberUpdateRepository) Save(ctx context.Context, item model.EmberUpdate) error {
	row := database.EmberUpdate{EmberID: item.EmberID, URL: item.URL, CheckedAt: item.CheckedAt, Available: item.Available, Error: item.Error}

	return r.db.WithContext(ctx).Omit("Ember").Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "ember_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"url", "checked_at", "available", "error"}),
	}).Create(&row).Error
}
