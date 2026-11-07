package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strings"
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
		fields := map[string]any{"message": "Hello", "player": "Alex", "state": "ready"}
		if i == 1 {
			fields["player"], fields["state"] = "Sam", "warning"
		}

		frame := protocol.GameFrame{ID: fmt.Sprint(i), ServerID: server.ID, Publication: model.GamePublication{Version: 1, Event: &model.GameEvent{ID: fmt.Sprint(i), Type: "game.announcement", Title: fmt.Sprintf("Event %d", i), OccurredAt: time.Now().UTC(), Data: fields}}}
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

	filtered := read("?search=alex&data.state=ready&data.player=Alex&limit=1")
	if len(filtered.Items) != 1 || filtered.Items[0].EventID != "2" || filtered.NextBefore == 0 {
		t.Fatal("combined game event filters ignored", filtered)
	}

	if older := read(fmt.Sprintf("?search=alex&data.state=ready&data.player=Alex&limit=1&before=%d", filtered.NextBefore)); len(older.Items) != 1 || older.Items[0].EventID != "0" || older.NextBefore != 0 {
		t.Fatal("filtered cursor lost an older event", older)
	}

	if state := read("?data.state=warning"); len(state.Items) != 1 || state.Items[0].EventID != "1" {
		t.Fatal("mod state filter was ignored", state)
	}

	if empty := read("?data.unknown="); len(empty.Items) != 0 {
		t.Fatal("missing field matched an existence filter", empty)
	}

	optionsResponse := request(f.admin, "GET", endpoint+"/events/filters", "")

	var options repository.GameEventFilterOptions

	if optionsResponse.Code != http.StatusOK || json.Unmarshal(optionsResponse.Body.Bytes(), &options) != nil ||
		!reflect.DeepEqual(options.Types, []string{"game.announcement"}) ||
		!reflect.DeepEqual(options.Fields["player"], []string{"Alex", "Sam"}) ||
		!reflect.DeepEqual(options.Fields["state"], []string{"ready", "warning"}) {
		t.Fatal("filter options were incorrect", optionsResponse.Code, optionsResponse.Body)
	}

	for _, query := range []string{"?limit=101", "?limit=0", "?before=-1", "?from=no", "?from=20&to=10", "?type=Invalid%20Type", "?data.=x", "?data.Bad%20name=x", "?data.player=Alex&data.player=Sam", "?search=%00", "?search=" + strings.Repeat("x", 1025), "?data.player=" + url.QueryEscape(strings.Repeat("x", 1025))} {
		expectAccess(t, f.admin, "GET", endpoint+"/events"+query, "", http.StatusBadRequest)
	}

	expectAccess(t, member, "GET", endpoint+"/events", "", http.StatusNotFound)
	expectAccess(t, member, "GET", endpoint+"/events/filters", "", http.StatusNotFound)
	expectAccess(t, f.admin, "POST", endpoint+"/members", `{"username":"game-events","role_id":5}`, http.StatusOK)
	expectAccess(t, member, "GET", endpoint+"/events", "", http.StatusOK)
	expectAccess(t, member, "GET", endpoint+"/events/filters", "", http.StatusOK)
	expectAccess(t, f.admin, "DELETE", endpoint+"/members/"+fmt.Sprint(user.ID), "", http.StatusNoContent)
	expectAccess(t, member, "GET", endpoint+"/events", "", http.StatusNotFound)
	expectAccess(t, member, "GET", endpoint+"/events/filters", "", http.StatusNotFound)
}
