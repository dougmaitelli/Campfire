package database

import "campfire/internal/shared/model"

// MonitoringAlert retains a raised or recovered condition independently of its
// inbox recipients and delivery outcomes. AlertState remains the current latch.
type MonitoringAlert struct {
	ID         string                     `gorm:"type:text;primaryKey" json:"id"`
	TargetType model.NotificationTarget   `gorm:"type:text;not null;index:monitoring_alert_target,priority:1" json:"target_type"`
	TargetID   string                     `gorm:"type:text;not null;index:monitoring_alert_target,priority:2" json:"target_id"`
	Code       string                     `gorm:"type:text;not null" json:"code"`
	Kind       model.NotificationKind     `gorm:"type:text;not null" json:"kind"`
	Severity   model.NotificationSeverity `gorm:"type:text;not null" json:"severity"`
	Title      string                     `gorm:"type:text;not null" json:"title"`
	Body       string                     `gorm:"type:text;not null" json:"body"`
	CreatedAt  int64                      `gorm:"not null;autoCreateTime:false;index" json:"created_at"`
}
