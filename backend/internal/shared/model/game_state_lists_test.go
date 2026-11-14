package model

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"campfire/internal/shared/limits"
)

func TestGameStateListValidation(t *testing.T) {
	for _, test := range []struct {
		name  string
		value any
		valid bool
	}{
		{"names", []any{"Alex", "Sam"}, true},
		{"empty", []any{}, true},
		{"mixed scalars", []any{"Alex", float64(0), false}, true},
		{"maximum items", repeatedGameList("Alex", limits.GameFieldListItems), true},
		{"too many items", repeatedGameList("Alex", limits.GameFieldListItems+1), false},
		{"maximum string bytes", []any{strings.Repeat("a", limits.GameFieldStringLength)}, true},
		{"too many string bytes", []any{strings.Repeat("界", 342)}, false},
		{"nested list", []any{[]any{"Alex"}}, false},
		{"object", []any{map[string]any{"name": "Alex"}}, false},
		{"null item", []any{nil}, false},
		{"null list", []any(nil), false},
		{"infinity", []any{math.Inf(1)}, false},
		{"NaN", []any{math.NaN()}, false},
		{"envelope limit", repeatedGameList(strings.Repeat("a", 1024), 17), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := &GameState{Fields: map[string]any{"player_names": test.value}, FieldLabels: GameFieldLabels{"player_names": "Online players"}}

			publication := GamePublication{Version: 1, State: state}
			if err := publication.Validate(); (err == nil) != test.valid {
				t.Fatalf("valid=%v: %v", test.valid, err)
			}

			if test.valid {
				data, err := json.Marshal(publication)
				if err != nil {
					t.Fatal(err)
				}

				var decoded GamePublication
				if err := json.Unmarshal(data, &decoded); err != nil {
					t.Fatal(err)
				}

				if !decoded.State.HasListFields() || decoded.Validate() != nil {
					t.Fatal("list did not survive JSON round trip")
				}
			}
		})
	}
}

func repeatedGameList(value any, count int) []any {
	list := make([]any, count)
	for i := range list {
		list[i] = value
	}

	return list
}

func TestGameEventsRemainScalarOnly(t *testing.T) {
	event := &GameEvent{ID: "one", Type: "player.joined", Title: "Joined", OccurredAt: time.Unix(1, 0), Data: map[string]any{"players": []any{"Alex"}}}
	if (GamePublication{Version: 1, Event: event}).Validate() == nil {
		t.Fatal("event arrays accepted")
	}

	event.Data["players"] = "Alex"
	if err := (GamePublication{Version: 1, Event: event}).Validate(); err != nil {
		t.Fatal(err)
	}
}
