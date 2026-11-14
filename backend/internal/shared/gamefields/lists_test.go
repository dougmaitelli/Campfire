package gamefields

import (
	"reflect"
	"testing"

	"campfire/internal/shared/model"
)

func TestListPresentation(t *testing.T) {
	state := &model.GameState{Fields: map[string]any{
		"player_names": []any{"Alex", "Sam", "Alex"},
		"mixed":        []any{"Round", float64(1234), true, false, "  "},
		"empty":        []any{},
		"blank":        []any{"", " "},
	}, FieldLabels: model.GameFieldLabels{"player_names": "Online players"}}
	ids := []string{"field.player_names", "field.mixed", "field.empty", "field.blank", "field.missing"}

	want := []Field{
		{ID: ids[0], Name: "Online players", Value: "Alex, Sam, Alex", Available: true},
		{ID: ids[1], Name: "mixed", Value: "Round, 1,234, Yes, No", Available: true},
		{ID: ids[2], Name: "empty", Value: "None", Available: true},
		{ID: ids[3], Name: "blank", Value: "None", Available: true},
		{ID: ids[4], Name: "missing"},
	}
	if got := Present(state, ids, state.FieldLabels, nil); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}

	if got := Present(nil, ids[:1], state.FieldLabels, nil); got[0].Available || got[0].Value != "" {
		t.Fatal("missing state exposed a list", got)
	}

	translated := Present(state, []string{"field.empty"}, nil, func(key string, _ map[string]string) string {
		if key != "gameState.emptyList" {
			t.Fatalf("unexpected key %q", key)
		}

		return "Empty list"
	})
	if translated[0].Value != "Empty list" {
		t.Fatal("empty list was not translated", translated)
	}
}
