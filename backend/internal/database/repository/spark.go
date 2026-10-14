// Package repository provides model-specific persistence operations.
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

// Repository errors distinguish missing records and uniqueness conflicts.
var (
	ErrNotFound = errors.New("spark not found")
	ErrConflict = errors.New("a Spark with that name or a local Spark already exists")
)

// SparkRepository persists Spark records using GORM.
type SparkRepository struct{ db *gorm.DB }

// NewSparkRepository creates a repository using the shared database pool.
func NewSparkRepository(db *gorm.DB) *SparkRepository {
	return &SparkRepository{db: db}
}

// Create inserts a Spark, enforcing unique names and a single local node.
func (r *SparkRepository) Create(ctx context.Context, spark model.Spark) error {
	record := database.Spark{
		ID: spark.ID, Name: spark.Name, Kind: string(spark.Kind),
		CreatedAt: spark.CreatedAt.Format(time.RFC3339Nano),
	}
	if err := r.db.WithContext(ctx).Create(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return ErrConflict
		}

		return fmt.Errorf("insert Spark: %w", err)
	}

	return nil
}

// List returns Sparks in creation order.
func (r *SparkRepository) List(ctx context.Context) ([]model.Spark, error) {
	var records []database.Spark
	if err := r.db.WithContext(ctx).Order("created_at").Order("id").Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list Sparks: %w", err)
	}

	sparks := make([]model.Spark, 0, len(records))
	for _, record := range records {
		spark, err := toSpark(record)
		if err != nil {
			return nil, err
		}

		sparks = append(sparks, spark)
	}

	return sparks, nil
}

// Get looks up a Spark by its opaque identifier.
func (r *SparkRepository) Get(ctx context.Context, id string) (model.Spark, error) {
	var record database.Spark
	if err := r.db.WithContext(ctx).Where("id = ?", id).Take(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.Spark{}, ErrNotFound
		}

		return model.Spark{}, fmt.Errorf("read Spark: %w", err)
	}

	return toSpark(record)
}

func toSpark(record database.Spark) (model.Spark, error) {
	timestamp, err := time.Parse(time.RFC3339Nano, record.CreatedAt)
	if err != nil {
		return model.Spark{}, fmt.Errorf("read Spark timestamp: %w", err)
	}

	return model.Spark{
		ID: record.ID, Name: record.Name, Kind: model.SparkKind(record.Kind), CreatedAt: timestamp,
	}, nil
}
