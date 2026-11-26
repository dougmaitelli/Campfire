package repository

import (
	"context"
	"errors"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CrashRepository owns placement-fenced, bounded retained crash evidence.
type CrashRepository struct{ db *gorm.DB }

// NewCrashRepository uses the panel's existing database and server ownership.
func NewCrashRepository(db *gorm.DB) *CrashRepository { return &CrashRepository{db: db} }

// Observe accepts replayable snapshots without changing retained evidence.
func (r *CrashRepository) Observe(ctx context.Context, sparkID, serverID string, records []model.CrashRecord) error {
	if len(records) > model.CrashReplayLimit {
		return errors.New("too many crash records")
	}

	for _, record := range records {
		if err := record.Validate(); err != nil {
			return err
		}
	}

	if len(records) == 0 {
		return nil
	}

	now := time.Now()
	cutoff := now.Add(-model.CrashRetention).Unix()

	return database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		var server database.Server
		if err := tx.Take(&server, "id = ? AND spark_id = ?", serverID, sparkID).Error; err != nil {
			return err
		}

		for _, record := range records {
			// Do not resurrect expired evidence or retain unbounded future timestamps.
			if record.ExitedAt < cutoff || record.ExitedAt > now.Add(5*time.Minute).Unix() {
				continue
			}

			item := database.ServerCrash{ID: record.ID, ServerID: serverID, SparkID: sparkID, ExitedAt: record.ExitedAt, Title: record.Title(), Record: record}
			if err := tx.Omit("Server").Clauses(clause.OnConflict{DoNothing: true}).Create(&item).Error; err != nil {
				return err
			}
		}

		var expired []string
		if err := tx.Model(&database.ServerCrash{}).Where("server_id = ?", serverID).Order("exited_at DESC, id DESC").Offset(model.CrashHistoryLimit).Pluck("id", &expired).Error; err != nil {
			return err
		}

		if len(expired) > 0 {
			return tx.Where("server_id = ? AND id IN ?", serverID, expired).Delete(&database.ServerCrash{}).Error
		}

		return nil
	})
}

// Prune applies age retention even to offline servers with no new observations.
func (r *CrashRepository) Prune(ctx context.Context) error {
	cutoff := time.Now().Add(-model.CrashRetention).Unix()
	return r.db.WithContext(ctx).Where("exited_at < ?", cutoff).Delete(&database.ServerCrash{}).Error
}
