package repository_test

import (
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
)

func TestRegisteredStateLabelsSurviveServiceRestart(t *testing.T) {
	db, _ := resourceHistoryStore(t)
	if err := db.ORM().Model(&database.Server{}).Where("id = ?", "game").Update("created_at", time.Now().UTC().Format(time.RFC3339Nano)).Error; err != nil {
		t.Fatal(err)
	}

	makeService := func() *service.GameIntegrationService {
		return &service.GameIntegrationService{Servers: repository.NewServerRepository(db.ORM()), Events: service.NewGameEventService(repository.NewGameEventRepository(db.ORM()))}
	}
	games := makeService()
	send := func(publication model.GamePublication) {
		t.Helper()

		if err := games.Receive(t.Context(), "node", protocol.GameFrame{ID: "request", ServerID: "game", Publication: publication}); err != nil {
			t.Fatal(err)
		}
	}
	state := &model.GameState{Fields: map[string]any{"map": "world", "unlabeled": true}, FieldLabels: model.GameFieldLabels{"map": "Legacy"}}
	send(model.GamePublication{Version: 1, State: state})

	before := games.Snapshot("game", "node", true, time.Now().Unix()).UpdatedAt
	definition := &model.GameRegistration{Revision: 1, StateFields: model.GameFieldLabels{"map": "World name", "optional": "Optional"}}
	send(model.GamePublication{Version: 1, Registration: definition})

	view := games.Snapshot("game", "node", true, time.Now().Unix())
	if view.State.FieldLabels["map"] != "World name" || len(view.State.FieldLabels) != 1 || view.UpdatedAt != before {
		t.Fatal("registration failed to relabel current state without refreshing it", view)
	}

	definition.StateFields["map"] = "Caller mutation"

	if games.Snapshot("game", "node", true, time.Now().Unix()).State.FieldLabels["map"] != "World name" {
		t.Fatal("registration maps were not isolated")
	}

	games = makeService()
	// No registration replay or inline labels needed after panel reconnect/restart.
	state.FieldLabels = nil
	send(model.GamePublication{Version: 1, State: state})

	view = games.Snapshot("game", "node", true, time.Now().Unix())
	if view.State.FieldLabels["map"] != "World name" || len(view.State.FieldLabels) != 1 {
		t.Fatal("registration was not restored", view)
	}

	definition.Revision = 2
	definition.StateFields = model.GameFieldLabels{"map": "Map"}
	send(model.GamePublication{Version: 1, Registration: definition})

	if games.Snapshot("game", "node", true, time.Now().Unix()).State.FieldLabels["map"] != "Map" {
		t.Fatal("new revision not reflected in current state")
	}
}
