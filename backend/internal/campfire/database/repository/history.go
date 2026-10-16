package repository

import (
	"context"
	"strconv"
	"strings"
	"time"

	"campfire/internal/campfire/database"

	"gorm.io/gorm"
)

// PruneHistory removes a bounded batch of expired terminal records. A zero
// cutoff disables that collection. Active records and schedule definitions stay.
func (r *ActivityRepository) PruneHistory(ctx context.Context, auditBefore, operationsBefore time.Time) (bool, error) {
	if auditBefore.IsZero() && operationsBefore.IsZero() {
		return false, nil
	}

	const batch = 1000

	more := false
	err := database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		if !auditBefore.IsZero() {
			expired := tx.Model(&database.AuditEvent{}).Select("id").Where("created_at < ? AND outcome IN ?", auditBefore.Unix(), []string{"succeeded", "failed", "accepted", "interrupted", "cancelled", "skipped"}).Order("created_at, id").Limit(batch)

			result := tx.Where("id IN (?)", expired).Delete(&database.AuditEvent{})
			if result.Error != nil {
				return result.Error
			}

			more = result.RowsAffected == batch
		}

		if !operationsBefore.IsZero() {
			expired := tx.Model(&database.ScheduleRun{}).Select("id").Where("finished_at > 0 AND finished_at < ? AND state IN ?", operationsBefore.Unix(), []string{"succeeded", "failed", "interrupted", "cancelled", "skipped"}).Order("finished_at, id").Limit(batch)

			result := tx.Where("id IN (?)", expired).Delete(&database.ScheduleRun{})
			if result.Error != nil {
				return result.Error
			}

			more = more || result.RowsAffected == batch
		}

		return nil
	})

	return more, err
}

// historyBefore accepts a timestamp:ID cursor that remains usable after pruning
// its anchor row. Existing ID cursors can still refer to retained records.
func historyBefore(query *gorm.DB, model any, before string) (*gorm.DB, error) {
	if before == "" {
		return query, nil
	}

	if timestamp, id, ok := strings.Cut(before, ":"); ok {
		created, err := strconv.ParseInt(timestamp, 10, 64)
		if err != nil || created < 0 || len(id) == 0 || len(id) > 256 {
			return nil, ErrAccessInput
		}

		return query.Where("(created_at, id) < (?, ?)", created, id), nil
	}

	cursor := query.Session(&gorm.Session{NewDB: true}).Model(model).Select("created_at, id").Where("id = ?", before)

	return query.Where("(created_at, id) < (?)", cursor), nil
}
