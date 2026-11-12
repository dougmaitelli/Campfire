package model

import (
	"fmt"
	"strings"
	"testing"
)

func TestGameRegistrationValidation(t *testing.T) {
	valid := func() GameRegistration {
		return GameRegistration{Revision: 1, StateFields: GameFieldLabels{"map": "World"}, EventFields: map[string]GameFieldLabels{"player.joined": {"player": "Player name"}}}
	}

	for _, test := range []struct {
		name   string
		change func(*GameRegistration)
	}{
		{"zero revision", func(r *GameRegistration) { r.Revision = 0 }},
		{"unsafe revision", func(r *GameRegistration) { r.Revision = 9007199254740992 }},
		{"invalid event type", func(r *GameRegistration) { r.EventFields["Invalid Type"] = GameFieldLabels{} }},
		{"invalid field", func(r *GameRegistration) { r.StateFields["Bad field"] = "Bad" }},
		{"padded label", func(r *GameRegistration) { r.StateFields["map"] = " World" }},
		{"multiline label", func(r *GameRegistration) { r.EventFields["player.joined"]["player"] = "Player\nname" }},
		{"long label", func(r *GameRegistration) { r.StateFields["map"] = strings.Repeat("x", 129) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := valid()
			test.change(&r)

			if (GamePublication{Version: 1, Registration: &r}).Validate() == nil {
				t.Fatal("invalid registration accepted")
			}
		})
	}

	registration := valid()
	if err := (GamePublication{Version: 1, Registration: &registration}).Validate(); err != nil {
		t.Fatal(err)
	}

	if (GamePublication{Version: 1, Registration: &registration, State: &GameState{}}).Validate() == nil {
		t.Fatal("mixed registration and data accepted")
	}

	for i := range 32 {
		labels := GameFieldLabels{}
		for j := range 32 {
			labels[fmt.Sprintf("field%d", j)] = strings.Repeat("x", 128)
		}

		registration.EventFields[fmt.Sprintf("event%d", i)] = labels
	}

	delete(registration.EventFields, "player.joined")

	if (GamePublication{Version: 1, Registration: &registration}).Validate() == nil {
		t.Fatal("oversized registration accepted")
	}
}
