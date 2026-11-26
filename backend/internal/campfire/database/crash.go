package database

import "campfire/internal/shared/model"

// ServerCrash retains bounded crash evidence independently of resource samples.
type ServerCrash struct {
	ID       string            `gorm:"primaryKey;type:text"`
	ServerID string            `gorm:"type:text;not null;index:server_crash_time,priority:1"`
	Server   Server            `gorm:"constraint:OnDelete:CASCADE"`
	SparkID  string            `gorm:"type:text;not null"`
	ExitedAt int64             `gorm:"not null;index:server_crash_time,priority:2"`
	Title    string            `gorm:"type:text;not null"`
	Record   model.CrashRecord `gorm:"serializer:json;type:text;not null"`
}
