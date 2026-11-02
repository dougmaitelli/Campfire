package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"
)

func TestRuntimeStatusSnapshotUsesSharedCollectionAndFreshGrants(t *testing.T) {
	f := newAccessFixture(t)
	user, member := f.user("runtime-viewer", database.UserRole)
	_, _, input := serverFixture(t, f.admin)
	created := request(f.admin, "POST", "/api/v1/servers", configJSON(t, input))

	var server model.Server
	if created.Code != http.StatusCreated || json.Unmarshal(created.Body.Bytes(), &server) != nil {
		t.Fatal(created.Code, created.Body)
	}

	endpoint := "/api/v1/servers/" + server.ID
	expectAccess(t, f.admin, "POST", endpoint+"/members", `{"username":"runtime-viewer","role_id":5}`, http.StatusOK)

	if err := f.statuses.Collect(t.Context()); err != nil {
		t.Fatal(err)
	}

	before := f.runtime.calls.Load()

	for range 10 {
		response := request(member, "GET", "/api/v1/servers/status", "")

		var statuses []model.RuntimeStatus
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &statuses) != nil || len(statuses) != 1 || statuses[0].ServerID != server.ID || statuses[0].State != model.RuntimeStateNotInstalled || statuses[0].Logs != "" {
			t.Fatal("incorrect cached runtime snapshot", response.Code, response.Body)
		}
	}

	if f.runtime.calls.Load() != before {
		t.Fatal("browser reads contacted Spark")
	}

	expectAccess(t, f.admin, "DELETE", fmt.Sprintf("%s/members/%d", endpoint, user.ID), "", http.StatusNoContent)

	response := request(member, "GET", "/api/v1/servers/status", "")
	if response.Code != http.StatusOK || response.Body.String() != "[]\n" {
		t.Fatal("cached status survived grant revocation", response.Code, response.Body)
	}
}
