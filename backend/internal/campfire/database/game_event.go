package database

import "time"

// GameEvent is immutable game activity. Its identity is scoped to one server.
type GameEvent struct {
	ID         int64          `gorm:"primaryKey;autoIncrement;index:game_event_receipt,priority:2;index:game_event_type,priority:3" json:"id"`
	ServerID   string         `gorm:"type:text;not null;uniqueIndex:game_event_identity,priority:1;index:game_event_type,priority:1;index:game_event_receipt,priority:1;index:game_event_time,priority:1" json:"server_id"`
	Server     Server         `gorm:"foreignKey:ServerID;constraint:OnDelete:CASCADE" json:"-"`
	SparkID    string         `gorm:"type:text;not null" json:"spark_id"`
	EventID    string         `gorm:"type:text;not null;uniqueIndex:game_event_identity,priority:2" json:"event_id"`
	Type       string         `gorm:"type:text;not null;index:game_event_type,priority:2" json:"type"`
	Title      string         `gorm:"type:text;not null" json:"title"`
	OccurredAt time.Time      `gorm:"not null;index:game_event_time,priority:2" json:"occurred_at"`
	ReceivedAt int64          `gorm:"not null;index" json:"received_at"`
	Data       map[string]any `gorm:"serializer:json;type:text" json:"data"`
}
