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

// ErrLogAlertNotFound prevents disclosure of another server's issue identity.
var ErrLogAlertNotFound = errors.New("log alert not found")

// LogAlertRepository stores actionable errors and their dismissal state.
type LogAlertRepository struct{ db *gorm.DB }

// NewLogAlertRepository uses the panel's shared database and placement records.
func NewLogAlertRepository(db *gorm.DB) *LogAlertRepository { return &LogAlertRepository{db: db} }

// Observe fences observations by current placement and commits each detection
// together with exactly one notification. Replayed snapshots preserve dismissal.
func (r *LogAlertRepository) Observe(ctx context.Context, sparkID, serverID string, observation model.LogObservation, queue bool) error {
	if err := observation.Validate(); err != nil {
		return err
	}

	return database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		var server database.Server
		if err := tx.Take(&server, "id = ? AND spark_id = ?", serverID, sparkID).Error; err != nil {
			return err
		}

		ids := make([]string, 0, len(observation.Alerts))
		for _, detection := range observation.Alerts {
			ids = append(ids, detection.ID)
		}

		known := make(map[string]bool, len(ids))
		if len(ids) > 0 {
			var stored []string
			if err := tx.Model(&database.LogAlertIssue{}).Where("server_id = ? AND id IN ?", serverID, ids).Pluck("id", &stored).Error; err != nil {
				return err
			}

			for _, id := range stored {
				known[id] = true
			}
		}

		for _, detection := range observation.Alerts {
			if known[detection.ID] {
				continue
			}

			item := database.LogAlertIssue{ID: detection.ID, ServerID: serverID, SparkID: sparkID, ExecutionID: detection.ExecutionID, Phase: detection.Phase, DetectedAt: detection.DetectedAt, Detection: detection, Archived: observation.Executions[detection.Phase] != detection.ExecutionID}

			result := tx.Omit("Server").Clauses(clause.OnConflict{DoNothing: true}).Create(&item)
			if result.Error != nil {
				return result.Error
			}

			if result.RowsAffected == 0 {
				continue
			}

			severity := model.NotificationWarning
			if detection.Severity == model.LogError {
				severity = model.NotificationFailure
			}
			// Console excerpts can contain secrets. Notifications contain only
			// author-supplied guidance; evidence is permission-checked separately.
			notification := database.Notification{TargetType: model.NotificationServer, TargetID: serverID, Kind: model.NotificationAlert, Severity: severity, Title: server.Name + ": " + detection.Title, Body: detection.Message, CreatedAt: time.Now().Unix()}
			if err := emitNotification(tx, &notification, queue); err != nil {
				return err
			}
		}

		for phase, execution := range observation.Executions {
			if err := tx.Model(&database.LogAlertIssue{}).Where("server_id = ? AND phase = ? AND execution_id <> ? AND archived = false", serverID, phase, execution).Update("archived", true).Error; err != nil {
				return err
			}
		}

		// Keep deduplication records for every item still replayable by Spark.
		query := tx.Where("server_id = ? AND archived = true AND detected_at < ?", serverID, time.Now().Add(-30*24*time.Hour).Unix())
		if len(ids) > 0 {
			query = query.Where("id NOT IN ?", ids)
		}

		return query.Delete(&database.LogAlertIssue{}).Error
	})
}

// List returns active issues with evidence only when explicitly authorized.
func (r *LogAlertRepository) List(ctx context.Context, serverID string, evidence bool) ([]model.DetectedLogAlert, error) {
	var records []database.LogAlertIssue
	if err := r.db.WithContext(ctx).Where("server_id = ? AND archived = false AND dismissed_at = 0", serverID).Order("detected_at DESC, id").Limit(64).Find(&records).Error; err != nil {
		return nil, err
	}

	result := make([]model.DetectedLogAlert, 0, len(records))
	for _, record := range records {
		alert := record.Detection
		if !evidence {
			alert.Line = ""
		}

		result = append(result, alert)
	}

	return result, nil
}

// Dismiss acknowledges one occurrence for all authorized users of that server.
func (r *LogAlertRepository) Dismiss(ctx context.Context, serverID, id string) error {
	result := r.db.WithContext(ctx).Model(&database.LogAlertIssue{}).Where("id = ? AND server_id = ?", id, serverID).Update("dismissed_at", time.Now().Unix())
	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return ErrLogAlertNotFound
	}

	return nil
}
