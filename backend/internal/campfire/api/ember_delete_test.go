package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"
)

func TestEmberDeletionPermissionsAndPinnedServers(t *testing.T) {
	f := newAccessFixture(t)
	_, member := f.user("member", database.UserRole)
	_, ember, input := serverFixture(t, f.admin)
	endpoint := "/api/v1/embers/" + ember.ID
	expectAccess(t, member, "DELETE", endpoint, "", http.StatusForbidden)
	expectAccess(t, f.panel, "DELETE", endpoint, "", http.StatusUnauthorized)
	expectAccess(t, f.admin, "DELETE", endpoint, "", http.StatusNoContent)
	expectAccess(t, f.admin, "GET", endpoint, "", http.StatusNotFound)
	expectAccess(t, f.admin, "DELETE", endpoint, "", http.StatusNotFound)

	definition, err := json.Marshal(ember)
	if err != nil {
		t.Fatal(err)
	}

	response := request(f.admin, "POST", "/api/v1/embers", string(definition))
	if response.Code != http.StatusCreated {
		t.Fatal(response.Code, response.Body)
	}

	var pinned model.Ember

	if err := json.Unmarshal(response.Body.Bytes(), &pinned); err != nil {
		t.Fatal(err)
	}

	input.EmberID = pinned.ID
	expectAccess(t, f.admin, "POST", "/api/v1/servers", configJSON(t, input), http.StatusCreated)

	endpoint = "/api/v1/embers/" + pinned.ID
	expectAccess(t, f.admin, "DELETE", endpoint, "", http.StatusConflict)
	expectAccess(t, f.admin, "GET", endpoint, "", http.StatusOK)
}
