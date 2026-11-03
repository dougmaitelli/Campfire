package database

import "campfire/internal/shared/model"

// LogAlertIssue retains detections and acknowledgements independently of the
// Spark connection and the personal notification inbox.
type LogAlertIssue struct {
	ID          string                 `gorm:"type:text;primaryKey"`
	ServerID    string                 `gorm:"type:text;primaryKey;index:log_alert_server,priority:1"`
	Server      Server                 `gorm:"foreignKey:ServerID;constraint:OnDelete:CASCADE"`
	SparkID     string                 `gorm:"type:text;not null"`
	ExecutionID string                 `gorm:"type:text;not null"`
	Phase       model.LogPhase         `gorm:"type:text;not null"`
	DetectedAt  int64                  `gorm:"not null"`
	Detection   model.DetectedLogAlert `gorm:"serializer:json;type:text;not null"`
	DismissedAt int64                  `gorm:"not null;default:0;index:log_alert_server,priority:2"`
	Archived    bool                   `gorm:"not null;default:false;index:log_alert_server,priority:3"`
}
