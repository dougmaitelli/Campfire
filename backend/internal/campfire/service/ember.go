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

// EmberStore persists immutable game definitions.
type EmberStore interface {
	Create(context.Context, model.Ember) error
	List(context.Context) ([]model.Ember, error)
	Get(context.Context, string) (model.Ember, error)
	Delete(context.Context, string) error
}

// EmberService stores immutable native definitions, including converted Egg imports.
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

// Import validates a definition and assigns an immutable revision identity.
func (s *EmberService) Import(ctx context.Context, ember model.Ember) (model.Ember, error) {
	if err := validateEmber(&ember); err != nil {
		return model.Ember{}, err
	}

	ember.ID = rand.Text()

	if err := s.store.Create(ctx, ember); err != nil {
		return model.Ember{}, err
	}

	return ember, nil
}

func validateEmber(ember *model.Ember) error {
	ember.Name = strings.TrimSpace(ember.Name)
	if err := ember.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidEmber, err)
	}

	if len(ember.Variables) > 128 || len(ember.Images) > 32 || len(ember.RequiredPorts) > 32 || strings.TrimSpace(ember.Startup) == "" {
		return fmt.Errorf("%w: definition exceeds limits or has no startup command", ErrInvalidEmber)
	}

	return nil
}

// List returns imported definitions.
func (s *EmberService) List(ctx context.Context) ([]model.Ember, error) { return s.store.List(ctx) }

// Get returns a pinned definition.
func (s *EmberService) Get(ctx context.Context, id string) (model.Ember, error) {
	ember, err := s.store.Get(ctx, id)
	if errors.Is(err, repository.ErrEmberNotFound) {
		return model.Ember{}, ErrEmberNotFound
	}

	return ember, err
}
