package gamefields

import (
	"reflect"
	"testing"

	"campfire/internal/shared/model"
)

func TestPresentation(t *testing.T) {
	tests := []struct {
		name  string
		state *model.GameState
		ids   []string
		want  []Field
	}{
		{
			name:  "selected built-ins combine without duplicate capacity",
			state: &model.GameState{Players: new(0), Capacity: new(20)},
			ids:   []string{"capacity", "players"},
			want:  []Field{{ID: "players", Name: "Players", Value: "0 / 20", Available: true}},
		},
		{
			name:  "unselected capacity never enters the value",
			state: &model.GameState{Players: new(3), Capacity: new(20)},
			ids:   []string{"players"},
			want:  []Field{{ID: "players", Name: "Players", Value: "3", Available: true}},
		},
		{
			name:  "capacity can be selected independently",
			state: &model.GameState{Players: new(3), Capacity: new(0)},
			ids:   []string{"capacity"},
			want:  []Field{{ID: "capacity", Name: "Capacity", Value: "0", Available: true}},
		},
		{
			name:  "missing players does not suppress capacity",
			state: &model.GameState{Capacity: new(20)},
			ids:   []string{"players", "capacity"},
			want:  []Field{{ID: "players", Name: "Players"}, {ID: "capacity", Name: "Capacity", Value: "20", Available: true}},
		},
		{
			name: "missing state exposes no values",
			ids:  []string{"players", "capacity", "field.world_name"},
			want: []Field{{ID: "players", Name: "Players"}, {ID: "capacity", Name: "Capacity"}, {ID: "field.world_name", Name: "world_name"}},
		},
		{
			name: "labels, raw keys, and values retain their spelling",
			state: &model.GameState{
				Fields:      map[string]any{"world": "Meadows", "raw_key": "unchanged", "url": "https://example.test", "pvp": false, "round": float64(0), "empty": "  "},
				FieldLabels: model.GameFieldLabels{"world": "World name", "url": "Server URL", "round": "Round"},
			},
			ids: []string{"field.world", "field.raw_key", "field.url", "field.pvp", "field.round", "field.empty", "field.missing"},
			want: []Field{
				{ID: "field.world", Name: "World name", Value: "Meadows", Available: true},
				{ID: "field.raw_key", Name: "raw_key", Value: "unchanged", Available: true},
				{ID: "field.url", Name: "Server URL", Value: "https://example.test", Available: true},
				{ID: "field.pvp", Name: "pvp", Value: "No", Available: true},
				{ID: "field.round", Name: "Round", Value: "0", Available: true},
				{ID: "field.empty", Name: "empty"},
				{ID: "field.missing", Name: "missing"},
			},
		},
		{
			name:  "custom fields cannot override built-ins or trigger metric formatting",
			state: &model.GameState{Players: new(3), Capacity: new(20), Fields: map[string]any{"players": true, "capacity": true}, FieldLabels: model.GameFieldLabels{"players": "Player tracking", "capacity": "Has slots"}},
			ids:   []string{"players", "capacity", "field.players", "field.capacity"},
			want: []Field{
				{ID: "players", Name: "Players", Value: "3 / 20", Available: true},
				{ID: "field.players", Name: "Player tracking", Value: "Yes", Available: true},
				{ID: "field.capacity", Name: "Has slots", Value: "Yes", Available: true},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var labels model.GameFieldLabels
			if test.state != nil {
				labels = test.state.FieldLabels
			}

			if got := Present(test.state, test.ids, labels, nil); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("got %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestLabelsRemainVerbatim(t *testing.T) {
	for _, label := range []string{"World name", "Server.status", "Server URL", "Invite code"} {
		if got := Label("field.world", model.GameFieldLabels{"world": label}, nil); got != label {
			t.Errorf("label %q changed to %q", label, got)
		}
	}
}

func TestBuiltinTranslationsAndCombinationUseTheSameLocalizer(t *testing.T) {
	translate := func(key string, values map[string]string) string {
		switch key {
		case "gameState.players":
			return "Connected players"
		case "gameState.capacity":
			return "Player limit"
		case "gameState.playersWithCapacity":
			return values["players"] + " of " + values["capacity"]
		case "gameState.no":
			return "No"
		default:
			t.Fatalf("unexpected translation key %s", key)
			return ""
		}
	}
	state := &model.GameState{Players: new(3), Capacity: new(20), Fields: map[string]any{"pvp": false}}

	got := Present(state, []string{"players", "capacity", "field.pvp"}, nil, translate)
	if len(got) != 2 || got[0].Name != "Connected players" || got[0].Value != "3 of 20" || got[1].Value != "No" {
		t.Fatal(got)
	}

	capacity := Present(state, []string{"capacity"}, nil, translate)
	if capacity[0].Name != "Player limit" {
		t.Fatal(capacity)
	}
}

func TestUnavailableValuesKeepIndependentLabels(t *testing.T) {
	fields := Present(nil, []string{"field.world", "field.raw_key"}, model.GameFieldLabels{"world": "World name"}, nil)
	if fields[0].Name != "World name" || fields[0].Available || fields[1].Name != "raw_key" {
		t.Fatal(fields)
	}
}

func TestEnglishNumberFormattingPreservesGroupingAndPrecision(t *testing.T) {
	for _, test := range []struct {
		value any
		want  string
	}{
		{12345, "12,345"}, {12.5, "12.5"}, {1.23456789012345, "1.23456789012345"},
		{0.0000001, "0.0000001"}, {float64(1e21), "1,000,000,000,000,000,000,000"},
	} {
		if got := englishNumber(test.value); got != test.want {
			t.Errorf("format(%v) = %q, want %q", test.value, got, test.want)
		}
	}
}

func TestCatalogDefinitionsAreCompleteAndUnambiguous(t *testing.T) {
	seen := make(map[string]bool)
	for _, definition := range builtins {
		if seen[definition.ID] || definition.ID == "" || englishMessages[definition.LabelKey] == "" {
			t.Fatal("invalid definition", definition)
		}

		seen[definition.ID] = true
		if definition.CombineWith != "" {
			if _, ok := Builtin(definition.CombineWith); !ok || englishMessages[definition.CombinedValueKey] == "" {
				t.Fatal("invalid combined rendering", definition)
			}
		}

		state := &model.GameState{Players: new(1), Capacity: new(2)}
		if fields := Present(state, []string{definition.ID}, nil, nil); !fields[0].Available {
			t.Fatal("built-in has no value accessor", definition)
		}
	}
}
