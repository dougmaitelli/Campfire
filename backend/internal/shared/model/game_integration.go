package model

import (
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"time"
)

// GamePublicationLimit bounds local requests and forwarded publications.
const GamePublicationLimit = 16 << 10

var gameFieldName = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)

// GamePublication is a versioned snapshot or event, without a publisher-selected
// server identity. Spark derives identity from the container's private socket.
type GamePublication struct {
	Version int        `json:"version"`
	State   *GameState `json:"state,omitempty"`
	Event   *GameEvent `json:"event,omitempty"`
}

// GameState replaces the entire preceding snapshot. All fields are optional.
type GameState struct {
	Players  *int           `json:"players,omitempty"`
	Capacity *int           `json:"capacity,omitempty"`
	Fields   map[string]any `json:"fields,omitempty"`
}

// GameEvent IDs identify retries of the same event. Persistence and deduplication
// are deliberately deferred to the event-history implementation stage.
type GameEvent struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	OccurredAt time.Time      `json:"occurred_at"`
	Title      string         `json:"title"`
	Data       map[string]any `json:"data,omitempty"`
}

// Validate applies the same schema and size bounds at both ends of the wire.
func (p GamePublication) Validate() error {
	if p.Version != 1 || (p.State == nil) == (p.Event == nil) {
		return errors.New("version 1 requires exactly one state or event")
	}

	if s := p.State; s != nil {
		if s.Players != nil && (*s.Players < 0 || *s.Players > 1000000) || s.Capacity != nil && (*s.Capacity < 0 || *s.Capacity > 1000000) {
			return errors.New("invalid player count or capacity")
		}

		if !validGameFields(s.Fields) {
			return errors.New("invalid state fields")
		}
	}

	if e := p.Event; e != nil {
		if len(e.ID) < 1 || len(e.ID) > 128 || !gameFieldName.MatchString(e.Type) || e.OccurredAt.IsZero() || len(e.Title) < 1 || len(e.Title) > 256 || !validGameFields(e.Data) {
			return errors.New("invalid game event")
		}
	}

	data, err := json.Marshal(p)
	if err != nil || len(data) > GamePublicationLimit {
		return errors.New("game publication exceeds limits")
	}

	return nil
}

func validGameFields(fields map[string]any) bool {
	if len(fields) > 32 {
		return false
	}

	for key, value := range fields {
		if !gameFieldName.MatchString(key) {
			return false
		}

		switch v := value.(type) {
		case string:
			if len(v) > 1024 {
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
