package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"campfire/internal/database"
	"campfire/internal/model"

	"gorm.io/gorm"
)

// ErrEmberNotFound identifies an unknown game definition.
var ErrEmberNotFound = errors.New("ember not found")

// EmberRepository stores immutable native game definitions.
type EmberRepository struct{ db *gorm.DB }

// NewEmberRepository creates a definition repository.
func NewEmberRepository(db *gorm.DB) *EmberRepository { return &EmberRepository{db: db} }

// Create imports a new immutable definition.
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

// Get returns one immutable definition.
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
