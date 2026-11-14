package agent

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"

	"campfire/internal/shared/protocol"
)

func TestGameStateListsPublishOnlyWhenNegotiated(t *testing.T) {
	g := &gameIntegrations{authorize: func(id string) bool { return id == "game" }, slots: make(chan struct{}, 1), wake: make(chan struct{}, 1)}
	calls := 0

	g.attach(t.Context(), func(message protocol.Message) error {
		calls++
		return g.acknowledge(&protocol.GameAck{ID: message.Game.ID})
	})

	publish := func(body string, status int) {
		t.Helper()

		response := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/publish", strings.NewReader(body))
		g.publish(response, request, "game")

		if response.Code != status {
			t.Fatalf("got %d, want %d: %s", response.Code, status, response.Body.String())
		}
	}
	// Existing scalar publishers still work with an older panel.
	publish(`{"version":1,"state":{"fields":{"map":"World"}}}`, http.StatusNoContent)

	body := `{"version":1,"state":{"fields":{"player_names":["Alex","Sam"]}}}`
	publish(body, http.StatusServiceUnavailable)

	if calls != 1 {
		t.Fatal("list forwarded to older panel")
	}

	g.attach(t.Context(), func(message protocol.Message) error {
		calls++

		if message.Game.ServerID != "game" || !reflect.DeepEqual(message.Game.Publication.State.Fields["player_names"], []any{"Alex", "Sam"}) {
			t.Fatal("source or list lost", message.Game)
		}

		return g.acknowledge(&protocol.GameAck{ID: message.Game.ID})
	}, true, true, true)
	publish(body, http.StatusNoContent)
	publish(`{"version":1,"state":{"fields":{"player_names":[["Alex"]]}}}`, http.StatusBadRequest)
	publish(`{"version":1,"state":{"fields":{"player_names":[null]}}}`, http.StatusBadRequest)

	if calls != 2 {
		t.Fatal("invalid list forwarded")
	}

	store := &ConfigurationStore{}
	if slices.Contains(store.capabilities(), protocol.CapabilityGameStateLists) {
		t.Fatal("disabled integration advertised lists")
	}

	store.games = g
	if !slices.Contains(store.capabilities(), protocol.CapabilityGameStateLists) {
		t.Fatal("enabled integration did not advertise lists")
	}
}
