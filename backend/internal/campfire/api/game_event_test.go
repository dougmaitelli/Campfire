package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
)

func TestGameEventsPersistDeduplicateAndEnforceServerAccess(t *testing.T) {
	f := newAccessFixture(t)
	user, member := f.user("game-events", database.UserRole)
	_, _, input := serverFixture(t, f.admin)
	input.GameIntegration = true
	response := request(f.admin, "POST", "/api/v1/servers", configJSON(t, input))

	var server model.Server
	if response.Code != http.StatusCreated || json.Unmarshal(response.Body.Bytes(), &server) != nil {
		t.Fatal(response.Code, response.Body)
	}

	endpoint := "/api/v1/servers/" + server.ID

	state := protocol.GameFrame{ID: "state", ServerID: server.ID, Publication: model.GamePublication{Version: 1, State: &model.GameState{Players: new(7)}}}
	if err := f.games.Receive(t.Context(), server.SparkID, state); err != nil {
		t.Fatal(err)
	}

	updated := f.games.Snapshot(server.ID, server.SparkID, true, time.Now().Unix()).UpdatedAt
	for i := range 3 {
		frame := protocol.GameFrame{ID: fmt.Sprint(i), ServerID: server.ID, Publication: model.GamePublication{Version: 1, Event: &model.GameEvent{ID: fmt.Sprint(i), Type: "game.announcement", Title: fmt.Sprintf("Event %d", i), OccurredAt: time.Now().UTC(), Data: map[string]any{"message": "Hello"}}}}
		if err := f.games.Receive(t.Context(), server.SparkID, frame); err != nil {
			t.Fatal(err)
		}

		if err := f.games.Receive(t.Context(), server.SparkID, frame); err != nil {
			t.Fatal(err)
		}
	}

	read := func(path string) repository.GameEventPage {
		t.Helper()

		response := request(f.admin, "GET", endpoint+"/events"+path, "")

		var page repository.GameEventPage
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &page) != nil {
			t.Fatal(response.Code, response.Body)
		}

		return page
	}

	snapshot := f.games.Snapshot(server.ID, server.SparkID, true, time.Now().Unix())
	if snapshot.UpdatedAt != updated || snapshot.State == nil || *snapshot.State.Players != 7 {
		t.Fatal("durable events changed current game state", snapshot)
	}

	page := read("?limit=2")
	if len(page.Items) != 2 || page.NextBefore == 0 {
		t.Fatal("deduplication or pagination failed", page)
	}

	if next := read(fmt.Sprintf("?limit=2&before=%d", page.NextBefore)); len(next.Items) != 1 || next.NextBefore != 0 {
		t.Fatal("cursor skipped retained event", next)
	}

	if page := read("?type=player.joined"); len(page.Items) != 0 {
		t.Fatal("event type filter ignored", page)
	}

	for _, query := range []string{"?limit=101", "?limit=0", "?before=-1", "?from=no", "?from=20&to=10", "?type=Invalid%20Type"} {
		expectAccess(t, f.admin, "GET", endpoint+"/events"+query, "", http.StatusBadRequest)
	}

	expectAccess(t, member, "GET", endpoint+"/events", "", http.StatusNotFound)
	expectAccess(t, f.admin, "POST", endpoint+"/members", `{"username":"game-events","role_id":5}`, http.StatusOK)
	expectAccess(t, member, "GET", endpoint+"/events", "", http.StatusOK)
	expectAccess(t, f.admin, "DELETE", endpoint+"/members/"+fmt.Sprint(user.ID), "", http.StatusNoContent)
	expectAccess(t, member, "GET", endpoint+"/events", "", http.StatusNotFound)
}
