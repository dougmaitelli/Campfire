package model

import (
	"encoding/json"
	"testing"
)

func TestEmberConsoleCommands(t *testing.T) {
	for _, test := range []struct {
		name, field string
		enabled     bool
	}{
		{"omitted", "", true}, {"enabled", `,"console_commands":true`, true}, {"disabled", `,"console_commands":false`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			ember, server := emberReviewFixture(t)

			ember.Schema = 2
			if err := json.Unmarshal([]byte(`{"schema":2`+test.field+`}`), &ember); err != nil {
				t.Fatal(err)
			}

			if err := ember.Validate(); err != nil {
				t.Fatal(err)
			}

			if ember.SupportsConsoleCommands() != test.enabled {
				t.Fatal("incorrect capability default")
			}

			server.SetEmberStatus(ember)

			if server.ConsoleCommands != test.enabled || VisibleServer(server, ember).ConsoleCommands != test.enabled {
				t.Fatal("capability not exposed in server views")
			}

			var restored Ember

			data, err := json.Marshal(ember)
			if err != nil {
				t.Fatal(err)
			}

			if err := json.Unmarshal(data, &restored); err != nil {
				t.Fatal(err)
			}

			if restored.SupportsConsoleCommands() != test.enabled {
				t.Fatal("capability lost during round trip")
			}

			ember.Schema = 1
			if (ember.Validate() != nil) != (ember.ConsoleCommands != nil) {
				t.Fatal("explicit capability must require schema 2")
			}
		})
	}
}
