package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"
)

func TestExplicitEmberApplicationAndInstallerPermissions(t *testing.T) {
	f := newAccessFixture(t)
	user, owner := f.user("owner", database.UserRole)
	_, operator := f.user("operator", database.UserRole)
	_, original, input := serverFixture(t, f.admin)

	response := request(f.admin, "POST", "/api/v1/servers", configJSON(t, input))
	if response.Code != http.StatusCreated {
		t.Fatal(response.Code, response.Body)
	}

	var server model.Server
	if err := json.Unmarshal(response.Body.Bytes(), &server); err != nil {
		t.Fatal(err)
	}

	endpoint := "/api/v1/servers/" + server.ID
	siblingInput := input
	siblingInput.Name = "Unchanged server"

	siblingInput.Ports = append([]model.ServerPort(nil), input.Ports...)
	siblingInput.Ports[0].HostPort++

	response = request(f.admin, "POST", "/api/v1/servers", configJSON(t, siblingInput))
	if response.Code != http.StatusCreated {
		t.Fatal(response.Code, response.Body)
	}

	var sibling model.Server
	if err := json.Unmarshal(response.Body.Bytes(), &sibling); err != nil {
		t.Fatal(err)
	}

	expectAccess(t, f.admin, "PUT", endpoint+"/owner", fmt.Sprintf(`{"user_id":%d}`, user.ID), http.StatusOK)
	expectAccess(t, owner, "POST", endpoint+"/members", `{"username":"operator","role_id":5}`, http.StatusOK)

	revision := original
	revision.Startup = "sleep {{SECONDS}} # new revision"

	revision.Variables = append(append([]model.Variable(nil), original.Variables...), model.Variable{Name: "LEVEL", Default: "3", Editable: true})

	definition, err := json.Marshal(revision)
	if err != nil {
		t.Fatal(err)
	}

	response = request(f.admin, "POST", "/api/v1/embers", string(definition))
	if response.Code != http.StatusCreated {
		t.Fatal(response.Code, response.Body)
	}

	if err := json.Unmarshal(response.Body.Bytes(), &revision); err != nil {
		t.Fatal(err)
	}

	input.EmberID = revision.ID
	input.Variables = map[string]string{"SECONDS": "7200"}

	body, err := json.Marshal(struct {
		model.ServerConfiguration
		ExpectedEmberID string `json:"expected_ember_id"`
	}{input, original.ID})
	if err != nil {
		t.Fatal(err)
	}

	expectAccess(t, f.admin, "PUT", endpoint, configJSON(t, input), http.StatusBadRequest)
	expectAccess(t, owner, "POST", endpoint+"/ember", string(body), http.StatusForbidden)

	var invalid map[string]any
	if err := json.Unmarshal(body, &invalid); err != nil {
		t.Fatal(err)
	}

	invalid["image"] = "unapproved:latest"

	invalidBody, err := json.Marshal(invalid)
	if err != nil {
		t.Fatal(err)
	}

	expectAccess(t, f.admin, "POST", endpoint+"/ember", string(invalidBody), http.StatusBadRequest)
	response = request(f.admin, "GET", endpoint, "")

	var unchanged model.Server
	if err := json.Unmarshal(response.Body.Bytes(), &unchanged); err != nil {
		t.Fatal(err)
	}

	if unchanged.EmberID != original.ID {
		t.Fatal("rejected application changed the pin")
	}

	for _, action := range []string{"update", "reinstall"} {
		expectAccess(t, operator, "POST", endpoint+"/actions", fmt.Sprintf(`{"action":%q}`, action), http.StatusForbidden)
	}

	response = request(f.admin, "POST", endpoint+"/ember", string(body))
	if response.Code != http.StatusOK {
		t.Fatal(response.Code, response.Body)
	}

	var changed model.Server
	if err := json.Unmarshal(response.Body.Bytes(), &changed); err != nil {
		t.Fatal(err)
	}

	if changed.ID != server.ID || changed.OwnerUserID != user.ID || changed.EmberID != revision.ID || changed.Variables["SECONDS"] != "7200" || changed.Variables["LEVEL"] != "3" {
		t.Fatal("revision application lost configuration or ownership", changed)
	}

	expectAccess(t, f.admin, "POST", endpoint+"/ember", string(body), http.StatusConflict)
	response = request(f.admin, "GET", "/api/v1/embers/"+original.ID, "")

	var retained model.Ember

	if response.Code != http.StatusOK {
		t.Fatal(response.Code, response.Body)
	}

	if err := json.Unmarshal(response.Body.Bytes(), &retained); err != nil {
		t.Fatal(err)
	}

	if retained.Startup != original.Startup || len(retained.Variables) != len(original.Variables) {
		t.Fatal("original import changed")
	}

	response = request(f.admin, "GET", "/api/v1/servers/"+sibling.ID, "")
	if err := json.Unmarshal(response.Body.Bytes(), &unchanged); err != nil {
		t.Fatal(err)
	}

	if unchanged.EmberID != original.ID {
		t.Fatal("applying a revision changed another server")
	}

	for _, action := range []string{"update", "reinstall"} {
		expectAccess(t, owner, "POST", endpoint+"/actions", fmt.Sprintf(`{"action":%q}`, action), http.StatusAccepted)
	}
}
