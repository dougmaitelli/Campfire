// Package repository provides model-specific persistence operations.
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

// Repository errors distinguish missing records and uniqueness conflicts.
var (
	ErrNotFound   = errors.New("spark not found")
	ErrConflict   = errors.New("a Spark with that name already exists")
	ErrSparkInUse = errors.New("move or delete this Spark's servers and finish its transfers before removing it")
)

// SparkRepository persists Spark records using GORM.
type SparkRepository struct{ db *gorm.DB }

// NewSparkRepository creates a repository using the shared database pool.
func NewSparkRepository(db *gorm.DB) *SparkRepository {
	return &SparkRepository{db: db}
}

// Delete revokes the Spark identity only after all placements and active
// transfer reservations have been removed in the same write transaction.
func (r *SparkRepository) Delete(ctx context.Context, id string) error {
	return database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&database.Server{}).Where("spark_id = ?", id).Count(&count).Error; err != nil {
			return err
		}

		if count != 0 {
			return ErrSparkInUse
		}

		if err := tx.Model(&database.ServerTransfer{}).Where("active = ? AND (source_spark_id = ? OR target_spark_id = ?)", true, id, id).Count(&count).Error; err != nil {
			return err
		}

		if count != 0 {
			return ErrSparkInUse
		}

		result := tx.Where("id = ?", id).Delete(&database.Spark{})
		if result.Error != nil {
			return result.Error
		}

		if result.RowsAffected != 1 {
			return ErrNotFound
		}

		return nil
	})
}

// Create inserts a Spark, enforcing unique names.
func (r *SparkRepository) Create(ctx context.Context, spark model.Spark) error {
	record := database.Spark{
		ID: spark.ID, Name: spark.Name, Description: spark.Description, Disabled: spark.Disabled,
		GameAddress: spark.GameAddress, CPULimit: spark.CPULimit, MemoryMB: spark.MemoryMB, DiskMB: spark.DiskMB,
		PortAllocations: spark.PortAllocations,
		CreatedAt:       spark.CreatedAt.Format(time.RFC3339Nano),
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

// Update changes only supplied operator fields, including explicit false values.
func (r *SparkRepository) Update(ctx context.Context, id string, changes model.SparkChanges) (model.Spark, error) {
	fields := make(map[string]any)
	if changes.Description != nil {
		fields["description"] = *changes.Description
	}

	if changes.Disabled != nil {
		fields["disabled"] = *changes.Disabled
	}

	if changes.GameAddress != nil {
		fields["game_address"] = *changes.GameAddress
	}

	if changes.CPULimit != nil {
		fields["cpu_limit"] = *changes.CPULimit
	}

	if changes.MemoryMB != nil {
		fields["memory_mb"] = *changes.MemoryMB
	}

	if changes.DiskMB != nil {
		fields["disk_mb"] = *changes.DiskMB
	}

	if changes.PortAllocations != nil {
		data, err := json.Marshal(*changes.PortAllocations)
		if err != nil {
			return model.Spark{}, fmt.Errorf("encode allocations: %w", err)
		}

		fields["port_allocations"] = string(data)
	}

	var updated model.Spark

	err := database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		result := tx.Model(&database.Spark{}).Where("id = ?", id).Updates(fields)
		if result.Error != nil {
			return fmt.Errorf("update Spark: %w", result.Error)
		}

		if result.RowsAffected == 0 {
			return ErrNotFound
		}

		var spark database.Spark
		if err := tx.Where("id = ?", id).Take(&spark).Error; err != nil {
			return err
		}

		servers, err := reservedServers(tx, id, "")
		if err != nil {
			return err
		}

		if err := checkReservations(spark, servers); err != nil {
			return ErrSparkReservations
		}

		updated, err = (&SparkRepository{db: tx}).Get(ctx, id)

		return err
	})

	return updated, err
}

func toSpark(record database.Spark) (model.Spark, error) {
	timestamp, err := time.Parse(time.RFC3339Nano, record.CreatedAt)
	if err != nil {
		return model.Spark{}, fmt.Errorf("read Spark timestamp: %w", err)
	}

	allocations := record.PortAllocations
	if allocations == nil {
		allocations = make([]model.PortAllocation, 0)
	}

	return model.Spark{
		SparkSettings: model.SparkSettings{GameAddress: record.GameAddress, CPULimit: record.CPULimit, MemoryMB: record.MemoryMB, DiskMB: record.DiskMB, PortAllocations: allocations},
		ID:            record.ID, Name: record.Name, CreatedAt: timestamp,
		Description: record.Description, Disabled: record.Disabled,
	}, nil
}
