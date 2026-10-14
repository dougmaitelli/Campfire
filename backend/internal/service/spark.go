// Package service provides application behavior independent of HTTP handlers.
package service

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"campfire/internal/database/repository"
	"campfire/internal/model"
)

// Spark errors describe validation, lookup, and registration conflicts.
var (
	ErrInvalidSpark  = errors.New("name must contain 1–100 characters without control characters; kind must be local or remote")
	ErrSparkNotFound = errors.New("spark not found")
	ErrSparkConflict = errors.New("a Spark with that name or a local Spark already exists")
)

// SparkStore is the persistence boundary used by the service.
type SparkStore interface {
	Create(context.Context, model.Spark) error
	List(context.Context) ([]model.Spark, error)
	Get(context.Context, string) (model.Spark, error)
}

// SparkService validates and registers game server nodes.
type SparkService struct{ store SparkStore }

// CreateSparkInput contains the operator-supplied registration fields.
type CreateSparkInput struct {
	Name string          `json:"name"`
	Kind model.SparkKind `json:"kind"`
}

// NewSparkService constructs the Spark application service.
func NewSparkService(store SparkStore) *SparkService {
	return &SparkService{store: store}
}

// Create validates input and assigns the identifier and creation timestamp.
func (s *SparkService) Create(ctx context.Context, input CreateSparkInput) (model.Spark, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 100 ||
		strings.ContainsFunc(name, unicode.IsControl) ||
		(input.Kind != model.SparkLocal && input.Kind != model.SparkRemote) {
		return model.Spark{}, ErrInvalidSpark
	}

	spark := model.Spark{
		ID: rand.Text(), Name: name, Kind: input.Kind, CreatedAt: time.Now().UTC(),
	}
	if err := s.store.Create(ctx, spark); err != nil {
		if errors.Is(err, repository.ErrConflict) {
			return model.Spark{}, ErrSparkConflict
		}

		return model.Spark{}, err
	}

	return spark, nil
}

// List returns registered nodes.
func (s *SparkService) List(ctx context.Context) ([]model.Spark, error) {
	return s.store.List(ctx)
}

// Get returns a registered node or ErrSparkNotFound.
func (s *SparkService) Get(ctx context.Context, id string) (model.Spark, error) {
	spark, err := s.store.Get(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return model.Spark{}, ErrSparkNotFound
	}

	return spark, err
}
