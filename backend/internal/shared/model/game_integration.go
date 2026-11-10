package model

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"campfire/internal/shared/limits"
	"campfire/internal/shared/utils"
)

// GamePublicationLimit bounds local requests and forwarded publications.
const GamePublicationLimit = 16 << 10

// GamePublication is a versioned snapshot or event, without a publisher-selected
// server identity. Spark derives identity from the container's private socket.
type GamePublication struct {
	Version int        `json:"version"`
	State   *GameState `json:"state,omitempty"`
	Event   *GameEvent `json:"event,omitempty"`
}

// GameState replaces the entire preceding snapshot. All fields are optional.
type GameState struct {
	Players     *int            `json:"players,omitempty"`
	Capacity    *int            `json:"capacity,omitempty"`
	Fields      map[string]any  `json:"fields,omitempty"`
	FieldLabels GameFieldLabels `json:"field_labels,omitempty"`
}

// GameEvent IDs identify retries of the same event within one server's history.
type GameEvent struct {
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	OccurredAt  time.Time       `json:"occurred_at"`
	Title       string          `json:"title"`
	Data        map[string]any  `json:"data,omitempty"`
	FieldLabels GameFieldLabels `json:"field_labels,omitempty"`
}

// GameFieldLabels supplies optional display text without changing field identity.
type GameFieldLabels map[string]string

// Label falls back to the exact machine key for older or unlabeled publications.
func (labels GameFieldLabels) Label(key string) string {
	if label := labels[key]; label != "" {
		return label
	}

	return key
}

// Validate accepts bounded, single-line labels only for fields in this publication.
func (labels GameFieldLabels) Validate(fields map[string]any) error {
	if len(labels) > limits.GameFields {
		return errors.New("too many game field labels")
	}

	for key, label := range labels {
		if _, exists := fields[key]; !exists || !validGameFieldName(key) || label == "" || len(label) > limits.GameFieldLabelLength || !utf8.ValidString(label) || strings.TrimSpace(label) != label || strings.ContainsFunc(label, unicode.IsControl) {
			return errors.New("invalid game field label")
		}
	}

	return nil
}

// ValidGameEventType bounds namespaced event filter input.
func ValidGameEventType(value string) bool { return validGameFieldName(value) }

func validGameFieldName(value string) bool {
	return len(value) > 0 && len(value) <= limits.GameFieldNameLength && value[0] >= 'a' && value[0] <= 'z' && utils.IsValidIdentifier(value)
}

// Validate applies the same schema and size bounds at both ends of the wire.
func (p GamePublication) Validate() error {
	if p.Version != 1 || (p.State == nil) == (p.Event == nil) {
		return errors.New("version 1 requires exactly one state or event")
	}

	if s := p.State; s != nil {
		if s.Players != nil && (*s.Players < 0 || *s.Players > limits.GamePlayers) || s.Capacity != nil && (*s.Capacity < 0 || *s.Capacity > limits.GamePlayers) {
			return errors.New("invalid player count or capacity")
		}

		if !validGameFields(s.Fields) {
			return errors.New("invalid state fields")
		}

		if err := s.FieldLabels.Validate(s.Fields); err != nil {
			return err
		}
	}

	if e := p.Event; e != nil {
		if len(e.ID) < 1 || len(e.ID) > 128 || !ValidGameEventType(e.Type) || e.OccurredAt.IsZero() || len(e.Title) < 1 || len(e.Title) > 256 || !validGameFields(e.Data) {
			return errors.New("invalid game event")
		}

		if err := e.FieldLabels.Validate(e.Data); err != nil {
			return err
		}
	}

	data, err := json.Marshal(p)
	if err != nil || len(data) > GamePublicationLimit {
		return errors.New("game publication exceeds limits")
	}

	return nil
}

func validGameFields(fields map[string]any) bool {
	if len(fields) > limits.GameFields {
		return false
	}

	for key, value := range fields {
		if !validGameFieldName(key) {
			return false
		}

		switch v := value.(type) {
		case string:
			if len(v) > limits.GameFieldStringLength {
				return false
			}
		case float64:
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return false
			}
		case bool:
		default:
			return false
		}
	}

	return true
}
