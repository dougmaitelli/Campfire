package database

import "campfire/internal/shared/model"

// GameMedia stores bounded attachments with the event, including cascading retention.
// Bytes stay off the control websocket and are excluded from event-list queries.
type GameMedia struct {
	ID           int64     `gorm:"primaryKey" json:"-"`
	GameEventID  int64     `gorm:"not null;uniqueIndex:game_media_identity,priority:1;index" json:"-"`
	Event        GameEvent `gorm:"foreignKey:GameEventID;constraint:OnDelete:CASCADE" json:"-"`
	ServerID     string    `gorm:"not null;index" json:"-"`
	AttachmentID string    `gorm:"not null;uniqueIndex:game_media_identity,priority:2" json:"id"`
	Name         string    `gorm:"not null" json:"name"`
	ContentType  string    `gorm:"not null" json:"content_type"`
	Size         int64     `gorm:"not null" json:"size"`
	Digest       string    `gorm:"not null" json:"-"`
	Data         []byte    `gorm:"not null" json:"-"`
}

// Metadata is the public contract, independent of the storage representation.
func (m GameMedia) Metadata() model.GameMedia {
	return model.GameMedia{ID: m.AttachmentID, Name: m.Name, ContentType: m.ContentType, Size: m.Size}
}
