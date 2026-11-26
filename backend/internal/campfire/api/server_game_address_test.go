package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/statuspage"
	"campfire/internal/shared/model"
)

func TestServerPublicAddressUsesExistingConfigureAccessAndNeverChangesRuntime(t *testing.T) {
	f := newAccessFixture(t)
	owner, ownerHandler := f.user("address-owner", database.UserRole)
	_, unrelated := f.user("address-viewer", database.UserRole)
	_, ember, configuration := serverFixture(t, f.admin)
	ember.Schema = 2
	ember.RequiredPorts[0].Primary = true

	definition, err := json.Marshal(ember)
	if err != nil {
		t.Fatal(err)
	}

	expectAccess(t, f.admin, "PUT", "/api/v1/embers/"+ember.ID, string(definition), http.StatusCreated)

	configuration.GameAddress = "Initial.Example.test"
	response := request(f.admin, "POST", "/api/v1/servers", configJSON(t, configuration))

	var server model.Server
	if response.Code != http.StatusCreated || json.Unmarshal(response.Body.Bytes(), &server) != nil {
		t.Fatal(response.Code, response.Body)
	}

	expectAccess(t, f.admin, "PATCH", "/api/v1/sparks/"+server.SparkID, `{"game_address":"games.example.test"}`, http.StatusOK)
	configuration = server.ServerConfiguration
	endpoint := "/api/v1/servers/" + server.ID
	expectAccess(t, unrelated, "PUT", endpoint, configJSON(t, configuration), http.StatusNotFound)
	expectAccess(t, f.admin, "POST", endpoint+"/members", fmt.Sprintf(`{"username":"address-viewer","role_id":%d}`, database.OperatorRole), http.StatusOK)
	expectAccess(t, f.admin, "PUT", endpoint+"/owner", fmt.Sprintf(`{"user_id":%d}`, owner.ID), http.StatusOK)

	f.runtime.status = func(input model.RuntimeRequest) (model.RuntimeStatus, error) {
		return model.RuntimeStatus{ServerID: input.ServerID, State: model.RuntimeStateRunning, Ready: true}, nil
	}
	if err := f.statuses.Collect(t.Context()); err != nil {
		t.Fatal(err)
	}

	expectAccess(t, f.admin, "POST", "/api/v1/status-pages", fmt.Sprintf(`{"title":"Game","slug":"game-address","entries":[{"server_id":%q,"name":"Game"}]}`, server.ID), http.StatusCreated)
	calls := f.runtime.calls.Load()

	for _, address := range []string{"192.0.2.1", "192.0.2.1:25565", "2001:db8::1", "[2001:db8::1]:25565", "https://Play.Example.test:32100/Join?World=Case", "steam://connect/Play.Example.test:27015", "connect Play.Example.test:27015", ""} {
		configuration.GameAddress = address
		expectAccess(t, unrelated, "PUT", endpoint, configJSON(t, configuration), http.StatusForbidden)
		response = request(ownerHandler, "PUT", endpoint, configJSON(t, configuration))

		var updated model.Server
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &updated) != nil || updated.GameAddress != address || updated.EmberHash != server.EmberHash {
			t.Fatal("metadata edit failed", address, response.Code, response.Body)
		}

		public := request(f.panel, "GET", "/api/v1/public/status-pages/game-address", "")

		want := address
		if want == "" {
			want = "games.example.test:25565"
		}

		var page statuspage.PublicPage
		if public.Code != http.StatusOK || json.Unmarshal(public.Body.Bytes(), &page) != nil || len(page.Entries) != 1 || page.Entries[0].GameAddress != want {
			t.Fatal(public.Code, public.Body)
		}
	}

	for _, invalid := range []string{"hello\nworld", "hello\x00world", strings.Repeat("a", 2049)} {
		configuration.GameAddress = invalid
		expectAccess(t, ownerHandler, "PUT", endpoint, configJSON(t, configuration), http.StatusBadRequest)
	}

	if f.runtime.calls.Load() != calls {
		t.Fatal("display metadata edit contacted Spark", calls, f.runtime.calls.Load())
	}

	expectAccess(t, ownerHandler, "PATCH", "/api/v1/sparks/"+server.SparkID, `{"game_address":"unauthorized.example.test"}`, http.StatusForbidden)
}
