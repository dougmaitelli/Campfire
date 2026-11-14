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

// GamePublication is a versioned registration, snapshot or event, without a publisher-selected
// server identity. Spark derives identity from the container's private socket.
type GamePublication struct {
	Version      int               `json:"version"`
	Registration *GameRegistration `json:"registration,omitempty"`
	State        *GameState        `json:"state,omitempty"`
	Event        *GameEvent        `json:"event,omitempty"`
}

// GameState replaces the entire preceding snapshot. All fields are optional.
type GameState struct {
	Players     *int            `json:"players,omitempty"`
	Capacity    *int            `json:"capacity,omitempty"`
	Fields      map[string]any  `json:"fields,omitempty"`
	FieldLabels GameFieldLabels `json:"field_labels,omitempty"`
}

// HasListFields reports whether this snapshot needs list-value support.
func (s *GameState) HasListFields() bool {
	if s != nil {
		for _, value := range s.Fields {
			if _, ok := value.([]any); ok {
				return true
			}
		}
	}

	return false
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
	if err := labels.validateDefinitions(); err != nil {
		return err
	}

	for key := range labels {
		if _, exists := fields[key]; !exists {
			return errors.New("game field label has no corresponding field")
		}
	}

	return nil
}

func (labels GameFieldLabels) validateDefinitions() error {
	if len(labels) > limits.GameFields {
		return errors.New("too many game field labels")
	}

	for key, label := range labels {
		if !validGameFieldName(key) || label == "" || len(label) > limits.GameFieldLabelLength || !utf8.ValidString(label) || strings.TrimSpace(label) != label || strings.ContainsFunc(label, unicode.IsControl) {
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
	kinds := 0

	for _, present := range []bool{p.State != nil, p.Event != nil, p.Registration != nil} {
		if present {
			kinds++
		}
	}

	if p.Version != 1 || kinds != 1 {
		return errors.New("version 1 requires exactly one registration, state or event")
	}

	if p.Registration != nil {
		if err := p.Registration.Validate(); err != nil {
			return err
		}
	}

	if s := p.State; s != nil {
		if s.Players != nil && (*s.Players < 0 || *s.Players > limits.GamePlayers) || s.Capacity != nil && (*s.Capacity < 0 || *s.Capacity > limits.GamePlayers) {
			return errors.New("invalid player count or capacity")
		}

		if !validGameFields(s.Fields, true) {
			return errors.New("invalid state fields")
		}

		if err := s.FieldLabels.Validate(s.Fields); err != nil {
			return err
		}
	}

	if e := p.Event; e != nil {
		if len(e.ID) < 1 || len(e.ID) > 128 || !ValidGameEventType(e.Type) || e.OccurredAt.IsZero() || len(e.Title) < 1 || len(e.Title) > 256 || !validGameFields(e.Data, false) {
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

func validGameFields(fields map[string]any, allowLists bool) bool {
	if len(fields) > limits.GameFields {
		return false
	}

	for key, value := range fields {
		if !validGameFieldName(key) {
			return false
		}

		if list, ok := value.([]any); ok {
			if !allowLists || list == nil || len(list) > limits.GameFieldListItems {
				return false
			}

			for _, item := range list {
				if !validGameScalar(item) {
					return false
				}
			}
		} else if !validGameScalar(value) {
			return false
		}
	}

	return true
}

func validGameScalar(value any) bool {
	switch v := value.(type) {
	case string:
		return len(v) <= limits.GameFieldStringLength
	case float64:
		return !math.IsNaN(v) && !math.IsInf(v, 0)
	case bool:
		return true
	default:
		return false
	}
}
