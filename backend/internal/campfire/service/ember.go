package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

// Ember errors describe invalid definitions and unknown imports.
var (
	ErrInvalidEmber  = errors.New("invalid Ember definition")
	ErrEmberNotFound = errors.New("ember not found")
)

// EmberStore persists current game definitions.
type EmberStore interface {
	Create(context.Context, model.Ember) error
	List(context.Context) ([]model.Ember, error)
	Get(context.Context, string) (model.Ember, error)
	Delete(context.Context, string) error
	Replace(context.Context, model.Ember) error
}

// EmberService stores current native definitions, including converted Egg imports.
type EmberService struct {
	store   EmberStore
	Updates *EmberUpdateService
}

// NewEmberService creates the game definition service.
func NewEmberService(store EmberStore) *EmberService { return &EmberService{store: store} }

// Delete removes an unused imported revision.
func (s *EmberService) Delete(ctx context.Context, id string) error {
	err := s.store.Delete(ctx, id)
	if errors.Is(err, repository.ErrEmberNotFound) {
		return ErrEmberNotFound
	}

	return err
}

// Import validates a definition and assigns a stable identity.
func (s *EmberService) Import(ctx context.Context, ember model.Ember) (model.Ember, error) {
	if err := validateEmber(&ember); err != nil {
		return model.Ember{}, err
	}

	ember.ID = rand.Text()
	ember.Revision = ""

	if err := s.store.Create(ctx, ember); err != nil {
		return model.Ember{}, err
	}

	ember.Revision = model.EmberHash(ember)

	return ember, nil
}

func validateEmber(ember *model.Ember) error {
	ember.Name = strings.TrimSpace(ember.Name)
	for index := range ember.RequiredPorts {
		ember.RequiredPorts[index].Description = strings.TrimSpace(ember.RequiredPorts[index].Description)
	}

	if err := ember.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidEmber, err)
	}

	if len(ember.Variables) > 128 || len(ember.Images) > 32 || len(ember.RequiredPorts) > 32 || strings.TrimSpace(ember.Startup) == "" && len(ember.StartupCommands) == 0 {
		return fmt.Errorf("%w: definition exceeds limits or has no startup command", ErrInvalidEmber)
	}

	return nil
}

// List returns imported definitions.
func (s *EmberService) List(ctx context.Context) ([]model.Ember, error) {
	items, err := s.store.List(ctx)
	for i := range items {
		items[i].Revision = model.EmberHash(items[i])
	}

	return items, err
}

// Get returns a pinned definition.
func (s *EmberService) Get(ctx context.Context, id string) (model.Ember, error) {
	ember, err := s.store.Get(ctx, id)
	if errors.Is(err, repository.ErrEmberNotFound) {
		return model.Ember{}, ErrEmberNotFound
	}

	ember.Revision = model.EmberHash(ember)

	return ember, err
}

// Replace updates the one local definition while retaining its stable identity.
func (s *EmberService) Replace(ctx context.Context, id string, ember model.Ember) (model.Ember, error) {
	if err := validateEmber(&ember); err != nil {
		return model.Ember{}, err
	}

	ember.ID, ember.Revision = id, ""
	if err := s.store.Replace(ctx, ember); err != nil {
		return model.Ember{}, err
	}

	ember.Revision = model.EmberHash(ember)

	return ember, nil
}
