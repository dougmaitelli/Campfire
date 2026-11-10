package database

import (
	"encoding/json"

	"campfire/internal/campfire/integrations"
	"campfire/internal/shared/model"
)

// Integration stores a provider connection; credentials are referenced, not stored here.
type Integration struct {
	ID            string          `gorm:"type:text;primaryKey"`
	Provider      string          `gorm:"type:text;not null;index"`
	Name          string          `gorm:"type:text;not null"`
	Enabled       bool            `gorm:"not null;default:false"`
	Config        json.RawMessage `gorm:"serializer:json;type:text;not null"`
	CredentialRef string          `gorm:"type:text;not null;default:''" json:"-"`
	CreatedAt     int64           `gorm:"not null;autoCreateTime:false"`
	UpdatedAt     int64           `gorm:"not null;autoUpdateTime:false"`
}

// ServerIntegration stores subscriptions and the identity of its persistent card.
type ServerIntegration struct {
	ID                   string                      `gorm:"type:text;primaryKey"`
	IntegrationID        string                      `gorm:"type:text;not null;uniqueIndex:server_integration_binding,priority:1"`
	Integration          Integration                 `gorm:"foreignKey:IntegrationID;constraint:OnDelete:CASCADE"`
	ServerID             string                      `gorm:"type:text;not null;index;uniqueIndex:server_integration_binding,priority:2"`
	Server               Server                      `gorm:"foreignKey:ServerID;constraint:OnDelete:CASCADE"`
	DestinationID        string                      `gorm:"type:text;not null;uniqueIndex:server_integration_binding,priority:3"`
	Enabled              bool                        `gorm:"not null;default:false"`
	Subscriptions        []integrations.Subscription `gorm:"serializer:json;type:text;not null"`
	Card                 integrations.CardSettings   `gorm:"serializer:json;type:text;not null"`
	MessageDestinationID string                      `gorm:"type:text;not null;default:''"`
	ExternalMessageID    string                      `gorm:"type:text;not null;default:''"`
	CreatedAt            int64                       `gorm:"not null;autoCreateTime:false"`
	UpdatedAt            int64                       `gorm:"not null;autoUpdateTime:false"`
}

// IntegrationMessage stores notification history, compact deduplication receipts,
// and singleton card work. Only terminal notification payloads can be compacted.
type IntegrationMessage struct {
	ID                int64                 `gorm:"primaryKey;autoIncrement;index:integration_message_retention,priority:3"`
	BindingID         string                `gorm:"type:text;not null;uniqueIndex:integration_message_event,priority:1;index:integration_message_retention,priority:1"`
	Binding           ServerIntegration     `gorm:"foreignKey:BindingID;constraint:OnDelete:CASCADE"`
	EventID           string                `gorm:"type:text;not null;uniqueIndex:integration_message_event,priority:2"`
	Category          integrations.Category `gorm:"type:text;not null;index:integration_message_category_due,priority:1"`
	DestinationID     string                `gorm:"type:text;not null"`
	Payload           json.RawMessage       `gorm:"serializer:json;type:text;not null"`
	State             model.DeliveryState   `gorm:"type:text;not null;index:integration_message_due,priority:1"`
	Attempts          int                   `gorm:"not null;default:0"`
	NextAttemptAt     int64                 `gorm:"not null;index:integration_message_due,priority:2;index:integration_message_category_due,priority:2"`
	DeliveredAt       int64                 `gorm:"not null;default:0"`
	ExternalMessageID string                `gorm:"type:text;not null;default:''"`
	Error             string                `gorm:"type:text;not null;default:''"`
	CreatedAt         int64                 `gorm:"not null;autoCreateTime:false;index:integration_message_retention,priority:2"`
}
