package database

import "campfire/internal/shared/model"

// ServerTransfer retains placement changes and their reservations across panel
// restarts. Historical rows survive server deletion, like execution history.
type ServerTransfer struct {
	ID                  string                    `gorm:"type:text;primaryKey" json:"id"`
	ServerID            string                    `gorm:"type:text;not null;uniqueIndex:transfers_active_server,where:active = 1" json:"server_id"`
	SourceSparkID       string                    `gorm:"type:text;not null;index" json:"source_spark_id"`
	TargetSparkID       string                    `gorm:"type:text;not null;index" json:"target_spark_id"`
	SourceConfiguration model.ServerConfiguration `gorm:"serializer:json;type:text;not null" json:"-"`
	TargetConfiguration model.ServerConfiguration `gorm:"serializer:json;type:text;not null" json:"-"`
	Phase               string                    `gorm:"type:text;not null;index" json:"phase"`
	ResumePhase         string                    `gorm:"type:text;not null;default:''" json:"-"`
	CancelFrom          string                    `gorm:"type:text;not null;default:''" json:"-"`
	Active              bool                      `gorm:"not null;index" json:"active"`
	Error               string                    `gorm:"type:text;not null;default:''" json:"error"`
	TransferredBytes    int64                     `gorm:"not null;default:0" json:"transferred_bytes"`
	TotalBytes          int64                     `gorm:"not null;default:0" json:"total_bytes"`
	Digest              string                    `gorm:"type:text;not null;default:''" json:"-"`
	CreatedAt           int64                     `gorm:"not null;autoCreateTime:false" json:"created_at"`
	UpdatedAt           int64                     `gorm:"not null;autoUpdateTime:false" json:"updated_at"`
	CanCancel           bool                      `gorm:"-" json:"can_cancel"`
}
