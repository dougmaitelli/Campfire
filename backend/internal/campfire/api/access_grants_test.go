package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"
)

func TestUserAndRoleAccessGrants(t *testing.T) {
	f := newAccessFixture(t)
	_, creator := f.user("creator", database.UserRole)
	_, viewer := f.user("viewer", database.UserRole)
	node, _, input := serverFixture(t, f.admin)
	sparkAccess := "/api/v1/sparks/" + node.ID + "/access"

	expectAccess(t, creator, "POST", "/api/v1/servers", configJSON(t, input), http.StatusForbidden)
	expectAccess(t, f.admin, "POST", sparkAccess+"/grants", `{"username":"creator","access_role_id":2}`, http.StatusNoContent)

	response := request(creator, "GET", "/api/v1/provisioning", "")
	if response.Code != http.StatusOK {
		t.Fatal("direct grant did not enable creation", response.Code, response.Body)
	}

	response = request(creator, "POST", "/api/v1/servers", configJSON(t, input))
	if response.Code != http.StatusCreated {
		t.Fatal(response.Code, response.Body)
	}

	var server model.Server
	if err := json.Unmarshal(response.Body.Bytes(), &server); err != nil {
		t.Fatal(err)
	}

	endpoint := "/api/v1/servers/" + server.ID
	expectAccess(t, viewer, "GET", endpoint, "", http.StatusNotFound)
	expectAccess(t, creator, "POST", endpoint+"/access/grants", `{"role_id":3,"access_role_id":5}`, http.StatusNoContent)
	expectAccess(t, viewer, "GET", endpoint, "", http.StatusOK)
	response = request(viewer, "GET", "/api/v1/servers", "")

	var visible []model.Server
	if err := json.Unmarshal(response.Body.Bytes(), &visible); err != nil || len(visible) != 1 {
		t.Fatal("role grant missing from server list", response.Body, err)
	}

	expectAccess(t, creator, "POST", endpoint+"/access/grants", `{"username":"viewer","role_id":3,"access_role_id":5}`, http.StatusBadRequest)
	expectAccess(t, creator, "POST", endpoint+"/access/grants", `{"username":"viewer","access_role_id":5}`, http.StatusNoContent)
	response = request(creator, "GET", endpoint+"/access", "")

	var access struct {
		Grants []struct {
			ID     int64
			RoleID *int64 `json:"role_id"`
		}
	}
	if err := json.Unmarshal(response.Body.Bytes(), &access); err != nil || len(access.Grants) != 2 {
		t.Fatal(response.Body, err)
	}

	for _, grant := range access.Grants {
		if grant.RoleID != nil {
			expectAccess(t, creator, "DELETE", fmt.Sprintf("%s/access/grants/%d", endpoint, grant.ID), "", http.StatusNoContent)
		}
	}

	expectAccess(t, viewer, "GET", endpoint, "", http.StatusOK)

	for _, grant := range access.Grants {
		if grant.RoleID == nil {
			expectAccess(t, creator, "DELETE", fmt.Sprintf("%s/access/grants/%d", endpoint, grant.ID), "", http.StatusNoContent)
		}
	}

	expectAccess(t, viewer, "GET", endpoint, "", http.StatusNotFound)
	expectAccess(t, f.admin, "POST", sparkAccess+"/grants", `{"role_id":3,"access_role_id":2}`, http.StatusNoContent)
	expectAccess(t, viewer, "GET", "/api/v1/provisioning", "", http.StatusOK)
	expectAccess(t, viewer, "GET", "/api/v1/sparks", "", http.StatusForbidden)
}
