//go:build !production

package devmock

import (
	"campfire/internal/shared/model"
	"context"
	"errors"

	"campfire/internal/shared/sparkrelease"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type sparkClientRecord struct {
	SparkID string `gorm:"primaryKey"`
	Version string
	Pruned  bool
}

func seedSparkClients(ctx context.Context, db *gorm.DB) error {
	if err := db.WithContext(ctx).AutoMigrate(&sparkClientRecord{}); err != nil {
		return err
	}

	for _, row := range []sparkClientRecord{{SparkID: "mock-europe", Version: "v1.0.0"}, {SparkID: "mock-america", Version: "v1.1.0"}, {SparkID: "mock-offline", Version: "v1.0.0"}} {
		if err := db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
	}

	return nil
}

// LatestSparkVersion keeps local development independent from GitHub.
func (r *Runtime) LatestSparkVersion(ctx context.Context) (string, error) { return "v1.1.0", ctx.Err() }

// PruneSpark simulates cleanup without accessing the development host's Docker.
func (r *Runtime) PruneSpark(ctx context.Context, id string) (model.PruneResult, error) {
	if !r.Connected(id) {
		return model.PruneResult{}, errors.New("mock Spark is offline")
	}

	changed := r.db.WithContext(ctx).Model(&sparkClientRecord{}).Where("spark_id = ? AND pruned = ?", id, false).Update("pruned", true)
	if changed.Error != nil || changed.RowsAffected == 0 {
		return model.PruneResult{}, changed.Error
	}

	return model.PruneResult{Containers: 2, Images: 3, Volumes: 1}, nil
}

// ReinstallSpark simulates a forced replacement with the bundled build.
func (r *Runtime) ReinstallSpark(ctx context.Context, id, version string) error {
	if !r.Connected(id) || version != "v1.1.0" {
		return errors.New("mock Spark must be connected and select the bundled build")
	}

	return r.db.WithContext(ctx).Model(&sparkClientRecord{}).Where("spark_id = ?", id).Update("version", version).Error
}

// UpdateSpark simulates a verified upgrade and persists its reported version.
func (r *Runtime) UpdateSpark(ctx context.Context, id, version string) error {
	if !r.Connected(id) {
		return errors.New("mock Spark is offline")
	}

	_, current, _ := r.SparkFeatures(id)
	if version != "v1.1.0" || !sparkrelease.Newer(current, version) {
		return errors.New("no newer mock Spark release is available")
	}

	return r.db.WithContext(ctx).Save(&sparkClientRecord{SparkID: id, Version: version}).Error
}
