package api_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
)

func TestMonitoringGameStateAuthorizationAndInvalidation(t *testing.T) {
	f := newAccessFixture(t)
	_, member := f.user("game-observer", database.UserRole)
	_, _, input := serverFixture(t, f.admin)
	response := request(f.admin, "POST", "/api/v1/servers", configJSON(t, input))

	var server model.Server
	if response.Code != http.StatusCreated || json.Unmarshal(response.Body.Bytes(), &server) != nil {
		t.Fatal(response.Code, response.Body)
	}

	f.runtime.metrics = func(q model.RuntimeRequest) (model.RuntimeStatus, error) {
		if q.Action != "metrics" {
			return model.RuntimeStatus{ServerID: q.ServerID, State: "stopped"}, nil
		}

		return model.RuntimeStatus{ServerID: q.ServerID, Metrics: &model.ResourceUsage{ServerID: q.ServerID, State: "running", SampledAt: time.Now().Unix()}}, nil
	}
	if err := f.history.Collect(t.Context()); err != nil {
		t.Fatal(err)
	}

	frame := protocol.GameFrame{ID: "request", ServerID: server.ID, Publication: model.GamePublication{Version: 1, State: &model.GameState{Players: new(4), Fields: map[string]any{"map": "world"}}}}
	if err := f.games.Receive(t.Context(), server.SparkID, frame); err != nil {
		t.Fatal(err)
	}

	read := func(handler http.Handler) service.MonitoringSnapshot {
		t.Helper()

		response := request(handler, "GET", "/api/v1/monitoring", "")

		var snapshot service.MonitoringSnapshot
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &snapshot) != nil {
			t.Fatal(response.Code, response.Body)
		}

		return snapshot
	}
	if len(read(member).Servers) != 0 {
		t.Fatal("unassigned member read game state")
	}

	snapshot := read(f.admin)
	if len(snapshot.Servers) != 1 || snapshot.Servers[0].Game == nil || snapshot.Servers[0].Game.Status != "fresh" || *snapshot.Servers[0].Game.State.Players != 4 {
		t.Fatal("current game state missing", snapshot)
	}

	endpoint := "/api/v1/servers/" + server.ID
	expectAccess(t, f.admin, "POST", endpoint+"/members", `{"username":"game-observer","role_id":5}`, http.StatusOK)

	if read(member).Servers[0].Game == nil {
		t.Fatal("authorized member cannot read game state")
	}

	expectAccess(t, f.admin, "POST", endpoint+"/actions", `{"action":"restart"}`, http.StatusAccepted)

	view := read(f.admin).Servers[0].Game
	if view.State != nil || view.Status != "unavailable" {
		t.Fatal("restart exposed old state", view)
	}
}
