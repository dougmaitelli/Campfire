// Package integrations defines provider-independent panel integration data.
// These types are not part of the Spark or game-publication protocol.
package integrations

import (
	"encoding/json"
	"errors"
	"strings"

	"campfire/internal/shared/model"
	"campfire/internal/shared/utils"
)

// Category identifies a Campfire event source, independently of its destination.
type Category string

// Categories initially cover the existing server event sources.
const (
	GameEvents    Category = "game"
	PowerEvents   Category = "power"
	ConsoleAlerts Category = "console_alert"
	BackupEvents  Category = "backup"
)

// Connection contains public provider configuration and an opaque secret reference.
// Credential storage and provider-specific configuration validation belong to adapters.
type Connection struct {
	ID            string          `json:"id"`
	Provider      string          `json:"provider"`
	Name          string          `json:"name"`
	Enabled       bool            `json:"enabled"`
	Config        json.RawMessage `json:"config"`
	CredentialRef string          `json:"-"`
	CreatedAt     int64           `json:"created_at"`
	UpdatedAt     int64           `json:"updated_at"`
}

// EventRule scopes field conditions to one event type; an empty type matches all.
type EventRule struct {
	Type   string            `json:"type"`
	Fields map[string]string `json:"fields,omitempty"`
}

// Subscription ORs event rules within one category. No rules includes all events.
type Subscription struct {
	Category Category    `json:"category"`
	Rules    []EventRule `json:"rules,omitempty"`
}

// CardSettings independently selects whether a card and its optional fields appear.
type CardSettings struct {
	Enabled bool     `json:"enabled"`
	Fields  []string `json:"fields"`
}

// MessageReference retains the destination that actually owns an external message.
// A channel change must not lose the reference needed to clean up the old card.
type MessageReference struct {
	DestinationID string `json:"destination_id"`
	ID            string `json:"id"`
}

// Binding connects a game server to one provider connection and destination.
type Binding struct {
	ID            string           `json:"id"`
	IntegrationID string           `json:"integration_id"`
	ServerID      string           `json:"server_id"`
	DestinationID string           `json:"destination_id"`
	Enabled       bool             `json:"enabled"`
	Subscriptions []Subscription   `json:"subscriptions"`
	Card          CardSettings     `json:"card"`
	StatusMessage MessageReference `json:"status_message"`
	CreatedAt     int64            `json:"created_at"`
	UpdatedAt     int64            `json:"updated_at"`
}

// Message is a durable outgoing message, independent of a personal inbox entry.
// Payload is a provider-neutral event or versioned singleton card work.
type Message struct {
	ID                int64               `json:"id"`
	BindingID         string              `json:"binding_id"`
	EventID           string              `json:"event_id"`
	Category          Category            `json:"category"`
	DestinationID     string              `json:"destination_id"`
	Payload           json.RawMessage     `json:"payload"`
	State             model.DeliveryState `json:"state"`
	Attempts          int                 `json:"attempts"`
	NextAttemptAt     int64               `json:"next_attempt_at"`
	DeliveredAt       int64               `json:"delivered_at"`
	ExternalMessageID string              `json:"external_message_id"`
	Error             string              `json:"error"`
	CreatedAt         int64               `json:"created_at"`
}

// Validate bounds configuration without interpreting provider-specific keys.
func (c Connection) Validate() error {
	if !validID(c.ID) || !utils.IsValidIdentifier(c.Provider) || len(c.Provider) > 64 || strings.TrimSpace(c.Name) == "" || len(c.Name) > 256 || len(c.CredentialRef) > 1024 || !validObject(c.Config, 16<<10) {
		return errors.New("invalid integration connection")
	}

	return nil
}

// Validate checks persisted subscription and field selection structure.
func (b Binding) Validate() error {
	if !validID(b.ID) || !validID(b.IntegrationID) || !validID(b.ServerID) || !validID(b.DestinationID) || len(b.Subscriptions) > 16 || !validNames(b.Card.Fields, 32) {
		return errors.New("invalid integration binding")
	}

	seen := make(map[Category]bool, len(b.Subscriptions))
	for _, subscription := range b.Subscriptions {
		if !subscription.Category.Valid() || seen[subscription.Category] || len(subscription.Rules) > 128 {
			return errors.New("invalid integration subscriptions")
		}

		seen[subscription.Category] = true
		for _, rule := range subscription.Rules {
			if rule.Type != "" && !validNames([]string{rule.Type}, 1) || len(rule.Fields) > 32 {
				return errors.New("invalid integration event rule")
			}

			for field, value := range rule.Fields {
				if !utils.IsValidIdentifier(field) || len(field) > 64 || len(value) > 1024 {
					return errors.New("invalid integration subscription fields")
				}
			}
		}
	}

	return nil
}

// Valid accepts the event categories currently understood by Campfire.
func (c Category) Valid() bool {
	return c == GameEvents || c == PowerEvents || c == ConsoleAlerts || c == BackupEvents
}

// ValidateMessage bounds an outbox input before it enters a database transaction.
func ValidateMessage(bindingID, eventID string, category Category, payload json.RawMessage) error {
	if !validID(bindingID) || !validID(eventID) || !category.Valid() || !validObject(payload, 64<<10) {
		return errors.New("invalid integration message")
	}

	return nil
}

func validID(value string) bool {
	return strings.TrimSpace(value) != "" && len(value) <= 256
}

func validObject(value json.RawMessage, maximum int) bool {
	if len(value) == 0 || len(value) > maximum {
		return false
	}

	var object map[string]json.RawMessage

	return json.Unmarshal(value, &object) == nil && object != nil
}

func validNames(values []string, maximum int) bool {
	if len(values) > maximum {
		return false
	}

	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if !utils.IsValidIdentifier(value) || len(value) > 128 || seen[value] {
			return false
		}

		seen[value] = true
	}

	return true
}
