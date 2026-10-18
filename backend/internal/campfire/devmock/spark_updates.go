//go:build !production

package devmock

import (
	"context"
	"errors"

	"campfire/internal/shared/sparkrelease"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type sparkClientRecord struct {
	SparkID string `gorm:"primaryKey"`
	Version string
}

func seedSparkClients(ctx context.Context, db *gorm.DB) error {
	if err := db.WithContext(ctx).AutoMigrate(&sparkClientRecord{}); err != nil {
		return err
	}

	for _, row := range []sparkClientRecord{{"mock-europe", "v1.0.0"}, {"mock-america", "v1.1.0"}, {"mock-offline", "v1.0.0"}} {
		if err := db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
	}

	return nil
}

// LatestSparkVersion keeps local development independent from GitHub.
func (r *Runtime) LatestSparkVersion(ctx context.Context) (string, error) { return "v1.1.0", ctx.Err() }

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
