package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGamePublicationSchema(t *testing.T) {
	for _, test := range []struct {
		name  string
		body  string
		valid bool
	}{
		{"empty snapshot", `{"version":1,"state":{}}`, true},
		{"optional fields", `{"version":1,"state":{"players":0,"fields":{"map":"world","pvp":true,"day":12}}}`, true},
		{"state labels", `{"version":1,"state":{"fields":{"game_mode":"survival"},"field_labels":{"game_mode":"Game mode"}}}`, true},
		{"event labels", `{"version":1,"event":{"id":"e-1","type":"player.joined","occurred_at":"2026-10-24T00:00:00Z","title":"Joined","data":{"player":"Alex"},"field_labels":{"player":"Player name"}}}`, true},
		{"orphan label", `{"version":1,"state":{"field_labels":{"map":"World"}}}`, false},
		{"empty label", `{"version":1,"state":{"fields":{"map":"world"},"field_labels":{"map":""}}}`, false},
		{"padded label", `{"version":1,"state":{"fields":{"map":"world"},"field_labels":{"map":" World "}}}`, false},
		{"multiline label", `{"version":1,"state":{"fields":{"map":"world"},"field_labels":{"map":"World\nname"}}}`, false},
		{"long label", `{"version":1,"state":{"fields":{"map":"world"},"field_labels":{"map":"` + strings.Repeat("a", 129) + `"}}}`, false},
		{"unicode byte limit", `{"version":1,"state":{"fields":{"map":"world"},"field_labels":{"map":"` + strings.Repeat("界", 43) + `"}}}`, false},
		{"event", `{"version":1,"event":{"id":"e-1","type":"game.announcement","occurred_at":"2026-10-24T00:00:00Z","title":"Hello"}}`, true},
		{"missing payload", `{"version":1}`, false},
		{"two payloads", `{"version":1,"state":{},"event":{}}`, false},
		{"unknown version", `{"version":2,"state":{}}`, false},
		{"negative count", `{"version":1,"state":{"players":-1}}`, false},
		{"large capacity", `{"version":1,"state":{"capacity":1000001}}`, false},
		{"nested fields", `{"version":1,"state":{"fields":{"map":{"name":"world"}}}}`, false},
		{"null field", `{"version":1,"state":{"fields":{"map":null}}}`, false},
		{"bad field name", `{"version":1,"state":{"fields":{"Map":"world"}}}`, false},
		{"long field", `{"version":1,"state":{"fields":{"map":"` + strings.Repeat("a", 1025) + `"}}}`, false},
		{"missing event ID", `{"version":1,"event":{"type":"game.test","occurred_at":"2026-10-24T00:00:00Z","title":"Hello"}}`, false},
		{"missing event timestamp", `{"version":1,"event":{"id":"e-1","type":"game.test","title":"Hello"}}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var publication GamePublication
			if err := json.Unmarshal([]byte(test.body), &publication); err != nil {
				t.Fatal(err)
			}

			if err := publication.Validate(); (err == nil) != test.valid {
				t.Fatalf("valid=%v: %v", test.valid, err)
			}
		})
	}
}
