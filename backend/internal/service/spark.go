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
	ErrInvalidSpark       = errors.New("name must contain 1–100 characters without control characters; kind must be local or remote")
	ErrSparkNotFound      = errors.New("spark not found")
	ErrSparkConflict      = errors.New("a Spark with that name or a local Spark already exists")
	ErrInvalidDescription = errors.New("description must contain at most 1000 characters, without control characters except newlines and tabs")
	ErrEmptySparkUpdate   = errors.New("provide at least one mutable Spark field")
	ErrInvalidSettings    = errors.New("invalid Spark capacity or networking settings")
)

// SparkStore is the persistence boundary used by the service.
type SparkStore interface {
	Create(context.Context, model.Spark) error
	List(context.Context) ([]model.Spark, error)
	Get(context.Context, string) (model.Spark, error)
	Update(context.Context, string, model.SparkChanges) (model.Spark, error)
}

// SparkConnectivity supplies live runtime connection state.
type SparkConnectivity interface {
	Connected(string) bool
}

// SparkService validates and registers game server nodes.
type SparkService struct {
	store        SparkStore
	connectivity SparkConnectivity
}

// CreateSparkInput contains the operator-supplied registration fields.
type CreateSparkInput struct {
	model.SparkSettings
	Name        string          `json:"name"`
	Kind        model.SparkKind `json:"kind"`
	Description string          `json:"description"`
	Disabled    bool            `json:"disabled"`
}

// UpdateSparkInput contains mutable operator fields; nil means unchanged.
type UpdateSparkInput = model.SparkChanges

// NewSparkService constructs the Spark application service.
func NewSparkService(store SparkStore, connectivity SparkConnectivity) *SparkService {
	return &SparkService{store: store, connectivity: connectivity}
}

// Create validates input and assigns the identifier and creation timestamp.
func (s *SparkService) Create(ctx context.Context, input CreateSparkInput) (model.Spark, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 100 ||
		strings.ContainsFunc(name, unicode.IsControl) ||
		(input.Kind != model.SparkLocal && input.Kind != model.SparkRemote) {
		return model.Spark{}, ErrInvalidSpark
	}

	description, err := validateDescription(input.Description)
	if err != nil {
		return model.Spark{}, err
	}

	spark := model.Spark{
		SparkSettings: input.SparkSettings,
		ID:            rand.Text(), Name: name, Kind: input.Kind, CreatedAt: time.Now().UTC(),
		Description: description, Disabled: input.Disabled,
	}
	if err := validateSettings(&spark.SparkSettings); err != nil {
		return model.Spark{}, err
	}

	if err := s.store.Create(ctx, spark); err != nil {
		if errors.Is(err, repository.ErrConflict) {
			return model.Spark{}, ErrSparkConflict
		}

		return model.Spark{}, err
	}

	return s.withConnectivity(spark), nil
}

// List returns registered nodes.
func (s *SparkService) List(ctx context.Context) ([]model.Spark, error) {
	sparks, err := s.store.List(ctx)
	if err != nil {
		return nil, err
	}

	for i := range sparks {
		sparks[i] = s.withConnectivity(sparks[i])
	}

	return sparks, nil
}

// Get returns a registered node or ErrSparkNotFound.
func (s *SparkService) Get(ctx context.Context, id string) (model.Spark, error) {
	spark, err := s.store.Get(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return model.Spark{}, ErrSparkNotFound
	}

	return s.withConnectivity(spark), err
}

// Update changes description or disabled state without changing node identity.
func (s *SparkService) Update(ctx context.Context, id string, input UpdateSparkInput) (model.Spark, error) {
	if input.Description == nil && input.Disabled == nil && input.GameAddress == nil && input.CPULimit == nil && input.MemoryMB == nil && input.DiskMB == nil && input.PortAllocations == nil {
		return model.Spark{}, ErrEmptySparkUpdate
	}

	if input.Description != nil {
		description, err := validateDescription(*input.Description)
		if err != nil {
			return model.Spark{}, err
		}

		input.Description = &description
	}

	if err := validateChanges(&input); err != nil {
		return model.Spark{}, err
	}

	spark, err := s.store.Update(ctx, id, input)
	if errors.Is(err, repository.ErrNotFound) {
		return model.Spark{}, ErrSparkNotFound
	}

	return s.withConnectivity(spark), err
}

func (s *SparkService) withConnectivity(spark model.Spark) model.Spark {
	spark.Connected = !spark.Disabled && s.connectivity != nil && s.connectivity.Connected(spark.ID)
	return spark
}

func validateDescription(value string) (string, error) {
	value = strings.TrimSpace(value)
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > 1000 ||
		strings.ContainsFunc(value, func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' }) {
		return "", ErrInvalidDescription
	}

	return value, nil
}
