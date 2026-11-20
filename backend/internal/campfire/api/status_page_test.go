package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/statuspage"
	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
)

func TestPublicStatusPagesAreExplicitAnonymousAndAdminManaged(t *testing.T) {
	f := newAccessFixtureWithAudit(t, true)
	user, ordinary := f.user("status-viewer", database.CreatorRole)
	_, _, configuration := serverFixture(t, f.admin)
	configuration.Name = "private-server-name"
	configuration.GameIntegration = true
	configuration.GameAddress = "play.example.test:32100"
	created := request(f.admin, "POST", "/api/v1/servers", configJSON(t, configuration))

	var server model.Server
	if created.Code != http.StatusCreated || json.Unmarshal(created.Body.Bytes(), &server) != nil {
		t.Fatal(created.Code, created.Body)
	}

	var ember database.Ember
	if err := f.db.ORM().First(&ember, "id = ?", server.EmberID).Error; err != nil {
		t.Fatal(err)
	}

	ember.Definition.ArtURL = "/game-art/mock-block.svg"

	definition, err := json.Marshal(ember.Definition)
	if err != nil {
		t.Fatal(err)
	}

	if err := f.db.ORM().Model(&ember).Update("definition", string(definition)).Error; err != nil {
		t.Fatal(err)
	}

	f.runtime.metrics = func(input model.RuntimeRequest) (model.RuntimeStatus, error) {
		return model.RuntimeStatus{ServerID: input.ServerID, Metrics: &model.ResourceUsage{ServerID: input.ServerID, State: model.RuntimeStateRunning, SampledAt: time.Now().Unix()}}, nil
	}
	if err := f.history.Collect(t.Context()); err != nil {
		t.Fatal(err)
	}

	if err := f.games.Receive(t.Context(), server.SparkID, protocol.GameFrame{ServerID: server.ID, Publication: model.GamePublication{Version: 1, State: &model.GameState{Players: new(4), Capacity: new(20), Fields: map[string]any{"map": "world", "player_names": []any{"Alex", "Sam"}}, FieldLabels: model.GameFieldLabels{"map": "World name"}}}}); err != nil {
		t.Fatal(err)
	}

	f.runtime.status = func(input model.RuntimeRequest) (model.RuntimeStatus, error) {
		return model.RuntimeStatus{ServerID: input.ServerID, State: model.RuntimeStateRunning, Ready: true, Progress: "private diagnostics", Logs: "private console"}, nil
	}
	if err := f.statuses.Collect(t.Context()); err != nil {
		t.Fatal(err)
	}

	body := fmt.Sprintf(`{"title":"Community status","slug":"community","entries":[{"server_id":%q,"name":"Public game"}]}`, server.ID)
	expectAccess(t, f.admin, "PUT", "/api/v1/servers/"+server.ID+"/owner", fmt.Sprintf(`{"user_id":%d}`, user.ID), http.StatusOK)
	expectAccess(t, f.panel, "POST", "/api/v1/status-pages", body, http.StatusUnauthorized)
	expectAccess(t, ordinary, "POST", "/api/v1/status-pages", body, http.StatusForbidden)
	expectAccess(t, ordinary, "GET", "/api/v1/status-pages", "", http.StatusForbidden)
	_, limited := createTestAPIToken(t, f.admin, `["servers.manage_all","audit.view"]`)
	expectAccess(t, bearerAPI(f.panel, limited), "POST", "/api/v1/status-pages", body, http.StatusForbidden)
	_, view := previewAs(t, f.admin, user.ID)
	expectAccess(t, view, "POST", "/api/v1/status-pages", body, http.StatusForbidden)
	response := request(f.admin, "POST", "/api/v1/status-pages", body)

	var page statuspage.Page
	if response.Code != http.StatusCreated || json.Unmarshal(response.Body.Bytes(), &page) != nil || page.ID == "" {
		t.Fatal(response.Code, response.Body)
	}

	endpoint := "/api/v1/status-pages/" + page.ID
	expectAccess(t, ordinary, "PUT", endpoint, body, http.StatusForbidden)
	expectAccess(t, ordinary, "DELETE", endpoint, "", http.StatusForbidden)
	expectAccess(t, f.admin, "POST", "/api/v1/status-pages", body, http.StatusConflict)

	var before, after int64
	if err := f.db.ORM().Model(&database.AuditEvent{}).Count(&before).Error; err != nil {
		t.Fatal(err)
	}

	calls := f.runtime.calls.Load()

	public := request(f.panel, "GET", "/api/v1/public/status-pages/community", "")
	if public.Code != http.StatusOK || public.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(public.Code, public.Body, public.Header())
	}

	var snapshot statuspage.PublicPage
	if err := json.Unmarshal(public.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}

	if snapshot.CollectedAt <= 0 || snapshot.CollectedAt > time.Now().Unix() || snapshot.Title != "Community status" || len(snapshot.Entries) != 1 || snapshot.Entries[0].State != statuspage.Online || snapshot.Entries[0].Name != "Public game" || snapshot.Entries[0].ObservedAt <= 0 {
		t.Fatal(snapshot)
	}

	entry := snapshot.Entries[0]
	if entry.ArtURL != "/game-art/mock-block.svg" || entry.GameAddress != "play.example.test:32100" || entry.GameState == nil || entry.GameState.State == nil || entry.GameState.UpdatedAt <= 0 || *entry.GameState.State.Players != 4 || entry.GameState.State.Fields["map"] != "world" || entry.GameState.State.FieldLabels["map"] != "World name" {
		t.Fatal("public game details missing", entry)
	}

	for _, secret := range []string{server.ID, server.Name, server.SparkID, "private diagnostics", "private console", "server_id", "spark_id", "permissions", "ports", "valid_for_seconds", "fresh_for_seconds"} {
		if strings.Contains(public.Body.String(), secret) {
			t.Fatal("public response leaked private metadata", secret)
		}
	}

	for _, visitor := range []http.Handler{f.admin, bearerAPI(f.panel, "invalid-token")} {
		response := request(visitor, "GET", "/api/v1/public/status-pages/community", "")

		var same statuspage.PublicPage
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &same) != nil || len(same.Entries) != 1 || same.Entries[0].ID != snapshot.Entries[0].ID {
			t.Fatal("authentication changed public content", response.Code, response.Body)
		}
	}

	if f.runtime.calls.Load() != calls {
		t.Fatal("public read contacted runtime")
	}

	if err := f.db.ORM().Model(&database.AuditEvent{}).Count(&after).Error; err != nil || after != before {
		t.Fatal("public polling created audits", before, after, err)
	}
	// Public access does not grant any private access, even for the same server.
	expectAccess(t, f.panel, "GET", "/api/v1/servers/"+server.ID, "", http.StatusUnauthorized)
	expectAccess(t, f.panel, "GET", "/api/v1/public/status-pages/missing", "", http.StatusNotFound)
	expectAccess(t, f.panel, "GET", "/api/v1/public/status-pages/INVALID", "", http.StatusNotFound)

	// Artwork must not turn a standalone public page into a private API client.
	for _, unsafeArt := range []string{
		`https://panel.example.test/game-art\..\api/v1/servers/private/files`,
		"https://panel.example.test/game-art%5c..%5capi/v1/servers/private/files",
		`/game-art\..\api/v1/servers/private/files`,
	} {
		ember.Definition.ArtURL = unsafeArt

		definition, err := json.Marshal(ember.Definition)
		if err != nil {
			t.Fatal(err)
		}

		if err := f.db.ORM().Model(&ember).Update("definition", string(definition)).Error; err != nil {
			t.Fatal(err)
		}

		response := request(f.panel, "GET", "/api/v1/public/status-pages/community", "")

		var filtered statuspage.PublicPage
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &filtered) != nil || len(filtered.Entries) != 1 || filtered.Entries[0].ArtURL != "" {
			t.Fatal("unsafe public artwork survived projection", response.Code, response.Body)
		}
	}

	oldID := snapshot.Entries[0].ID

	if err := f.db.ORM().Model(&database.Spark{}).Where("id = ?", server.SparkID).Update("disabled", true).Error; err != nil {
		t.Fatal(err)
	}

	snapshot = statuspage.PublicPage{}

	disabled := request(f.panel, "GET", "/api/v1/public/status-pages/community", "")
	if err := json.Unmarshal(disabled.Body.Bytes(), &snapshot); err != nil || snapshot.Entries[0].State != statuspage.Unknown || snapshot.Entries[0].ObservedAt != 0 || snapshot.Entries[0].GameState != nil {
		t.Fatal("disabled Spark retained public availability", snapshot, err)
	}

	changed := strings.Replace(body, "community\"", "renamed\"", 1)
	expectAccess(t, f.admin, "PUT", endpoint, changed, http.StatusOK)
	expectAccess(t, f.panel, "GET", "/api/v1/public/status-pages/community", "", http.StatusNotFound)

	updated := request(f.panel, "GET", "/api/v1/public/status-pages/renamed", "")
	if err := json.Unmarshal(updated.Body.Bytes(), &snapshot); err != nil || snapshot.Entries[0].ID != oldID {
		t.Fatal("membership identity changed", snapshot, err)
	}

	expectAccess(t, f.admin, "PUT", endpoint, `{"title":"Empty","slug":"renamed","entries":[]}`, http.StatusOK)

	empty := request(f.panel, "GET", "/api/v1/public/status-pages/renamed", "")
	if err := json.Unmarshal(empty.Body.Bytes(), &snapshot); err != nil || len(snapshot.Entries) != 0 {
		t.Fatal(empty.Body, err)
	}

	expectAccess(t, f.admin, "DELETE", endpoint, "", http.StatusNoContent)
	expectAccess(t, f.panel, "GET", "/api/v1/public/status-pages/renamed", "", http.StatusNotFound)
	assertAuditOutcome(t, f, "status-page.create", model.AuditSucceeded, "status-page:"+page.ID)
	assertAuditOutcome(t, f, "status-page.delete", model.AuditSucceeded, "status-page:"+page.ID)
}

func TestStatusPageValidationDoesNotPartiallyPublish(t *testing.T) {
	f := newAccessFixtureWithAudit(t, true)
	for _, body := range []string{
		`{"title":"Empty slug","slug":"","entries":[]}`,
		`{"title":"Bad slug","slug":"../private","entries":[]}`,
		`{"title":"Mixed case","slug":"Community","entries":[]}`,
		`{"title":"Missing server","slug":"community","entries":[{"server_id":"missing","name":"Public"}]}`,
		`{"title":"Duplicate","slug":"community","entries":[{"server_id":"missing","name":"One"},{"server_id":"missing","name":"Two"}]}`,
	} {
		expectAccess(t, f.admin, "POST", "/api/v1/status-pages", body, http.StatusBadRequest)
	}

	expectAccess(t, f.admin, "DELETE", "/api/v1/status-pages/%0Aprivate-token", "", http.StatusNotFound)
	expectAccess(t, f.admin, "PUT", "/api/v1/status-pages/%0Aprivate-token", `{"title":"Valid","slug":"valid","entries":[]}`, http.StatusNotFound)

	var unsafeTargets int64
	if err := f.db.ORM().Model(&database.AuditEvent{}).Where("target LIKE ?", "%private-token%").Count(&unsafeTargets).Error; err != nil || unsafeTargets != 0 {
		t.Fatal("unsafe publication identifiers reached audit metadata", unsafeTargets, err)
	}

	listed := request(f.admin, "GET", "/api/v1/status-pages", "")
	if listed.Code != http.StatusOK || strings.TrimSpace(listed.Body.String()) != "[]" {
		t.Fatal(listed.Code, listed.Body)
	}
}
