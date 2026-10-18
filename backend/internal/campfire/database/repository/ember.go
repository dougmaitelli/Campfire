package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

// ErrEmberNotFound identifies an unknown game definition.
var ErrEmberNotFound = errors.New("ember not found")

// ErrEmberInUse prevents removing revisions still pinned by servers.
var ErrEmberInUse = errors.New("reassign or delete this Ember's servers before deleting it")

// EmberRepository stores current native game definitions.
type EmberRepository struct{ db *gorm.DB }

// NewEmberRepository creates a definition repository.
func NewEmberRepository(db *gorm.DB) *EmberRepository { return &EmberRepository{db: db} }

// Replace replaces the current definition while retaining its identity.
func (r *EmberRepository) Replace(ctx context.Context, ember model.Ember) error {
	return database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&database.ServerTransfer{}).Joins("JOIN servers ON servers.id = server_transfers.server_id").Where("server_transfers.active = true AND servers.ember_id = ?", ember.ID).Count(&count).Error; err != nil {
			return err
		}

		if count != 0 {
			return ErrTransferActive
		}

		ember.Revision = ""

		data, err := json.Marshal(ember)
		if err != nil {
			return err
		}

		result := tx.Model(&database.Ember{}).Where("id = ?", ember.ID).Updates(map[string]any{"name": ember.Name, "definition": string(data), "update_checked_at": time.Now().Unix(), "update_available": false, "update_error": ""})
		if result.Error == nil && result.RowsAffected == 0 {
			return ErrEmberNotFound
		}

		return result.Error
	})
}

// Delete removes an unused revision while preserving server references.
func (r *EmberRepository) Delete(ctx context.Context, id string) error {
	return database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		var count int64

		if err := tx.Model(&database.Server{}).Where("ember_id = ?", id).Count(&count).Error; err != nil {
			return err
		}

		if count > 0 {
			return ErrEmberInUse
		}

		result := tx.Where("id = ?", id).Delete(&database.Ember{})
		if errors.Is(result.Error, gorm.ErrForeignKeyViolated) {
			return ErrEmberInUse
		}

		if result.Error != nil {
			return result.Error
		}

		if result.RowsAffected == 0 {
			return ErrEmberNotFound
		}

		return nil
	})
}

// Create imports a new definition.
func (r *EmberRepository) Create(ctx context.Context, ember model.Ember) error {
	record := database.Ember{ID: ember.ID, Name: ember.Name, Definition: ember, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if err := r.db.WithContext(ctx).Create(&record).Error; err != nil {
		return fmt.Errorf("save Ember: %w", err)
	}

	return nil
}

// List returns imported definitions in name order.
func (r *EmberRepository) List(ctx context.Context) ([]model.Ember, error) {
	var records []database.Ember
	if err := r.db.WithContext(ctx).Order("name").Order("created_at").Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list Embers: %w", err)
	}

	embers := make([]model.Ember, 0, len(records))
	for _, record := range records {
		embers = append(embers, record.Definition)
	}

	return embers, nil
}

// Get returns one current definition.
func (r *EmberRepository) Get(ctx context.Context, id string) (model.Ember, error) {
	var record database.Ember
	if err := r.db.WithContext(ctx).Where("id = ?", id).Take(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.Ember{}, ErrEmberNotFound
		}

		return model.Ember{}, fmt.Errorf("read Ember: %w", err)
	}

	return record.Definition, nil
}
