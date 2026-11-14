package service

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
)

func TestGameStateListsAreIsolatedReplacedAndExpired(t *testing.T) {
	now := time.Unix(1000, 0)
	service := &GameIntegrationService{Servers: gameServerStore{}, now: func() time.Time { return now }}
	service.Observe("game", "owner", model.ResourceUsage{State: "running", StartedAt: 900}, now, false)

	names := []any{"Alex", "Sam"}

	frame := protocol.GameFrame{ID: "list", ServerID: "game", Publication: model.GamePublication{Version: 1, State: &model.GameState{Fields: map[string]any{"player_names": names}}}}
	if err := service.Receive(t.Context(), "owner", frame); err != nil {
		t.Fatal(err)
	}

	names[0] = "publisher mutation"

	view := service.Snapshot("game", "owner", true, now.Unix())
	if got := view.State.Fields["player_names"]; !reflect.DeepEqual(got, []any{"Alex", "Sam"}) {
		t.Fatal("publisher mutated snapshot", got)
	}

	returnedNames, ok := view.State.Fields["player_names"].([]any)
	if !ok {
		t.Fatal("snapshot list changed type")
	}

	returnedNames[1] = "reader mutation"

	if got := service.Snapshot("game", "owner", true, now.Unix()).State.Fields["player_names"]; !reflect.DeepEqual(got, []any{"Alex", "Sam"}) {
		t.Fatal("reader mutated snapshot", got)
	}

	frame.Publication.State.Fields["player_names"] = []any{}
	if err := service.Receive(t.Context(), "owner", frame); err != nil {
		t.Fatal(err)
	}

	encoded, err := json.Marshal(service.Snapshot("game", "owner", true, now.Unix()).State.Fields["player_names"])
	if err != nil || string(encoded) != "[]" {
		t.Fatalf("empty list lost during cloning: %s %v", encoded, err)
	}

	frame.Publication.State = &model.GameState{}
	if err := service.Receive(t.Context(), "owner", frame); err != nil {
		t.Fatal(err)
	}

	if len(service.Snapshot("game", "owner", true, now.Unix()).State.Fields) != 0 {
		t.Fatal("omitted list was retained")
	}

	frame.Publication.State.Fields = map[string]any{"player_names": []any{"Sam"}}
	if err := service.Receive(t.Context(), "owner", frame); err != nil {
		t.Fatal(err)
	}

	if view := service.Snapshot("game", "owner", true, now.Add(60*time.Second).Unix()); view.State != nil {
		t.Fatal("expired list exposed", view)
	}

	service.Invalidate("game")

	if view := service.Snapshot("game", "owner", true, now.Unix()); view.State != nil {
		t.Fatal("list survived invalidation", view)
	}
}
