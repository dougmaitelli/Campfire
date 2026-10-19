package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"
)

func TestHiddenVariablesArePrivateAndPreservedOnEdit(t *testing.T) {
	f := newAccessFixture(t)
	owner, regular := f.user("hidden-owner", database.UserRole)
	_, ember, input := serverFixture(t, f.admin)
	ember.Variables = append(ember.Variables,
		model.Variable{Name: "INTERNAL_SETTING", Default: "private-default", Editable: true, Hidden: true},
		model.Variable{Name: "INTERNAL_LOCKED", Default: "locked-default", Hidden: true},
	)
	data, err := json.Marshal(ember)
	if err != nil {
		t.Fatal(err)
	}
	expectAccess(t, f.admin, "PUT", "/api/v1/embers/"+ember.ID, string(data), http.StatusCreated)
	input.Variables = map[string]string{"INTERNAL_SETTING": "managed-private-value"}
	data, err = json.Marshal(struct {
		model.ServerConfiguration
		OwnerUserID int64 `json:"owner_user_id"`
	}{input, owner.ID})
	if err != nil {
		t.Fatal(err)
	}
	response := request(f.admin, "POST", "/api/v1/servers", string(data))
	if response.Code != http.StatusCreated {
		t.Fatal(response.Code, response.Body)
	}
	var server model.Server
	if err := json.Unmarshal(response.Body.Bytes(), &server); err != nil {
		t.Fatal(err)
	}
	endpoint := "/api/v1/servers/" + server.ID
	for _, path := range []string{"/api/v1/servers", endpoint, endpoint + "/editor"} {
		response := request(regular, "GET", path, "")
		if response.Code != http.StatusOK {
			t.Fatal(path, response.Code, response.Body)
		}
		for _, hidden := range []string{"INTERNAL_SETTING", "INTERNAL_LOCKED", "private-default", "managed-private-value", "locked-default"} {
			if strings.Contains(response.Body.String(), hidden) {
				t.Fatal("hidden variable exposed", path, hidden)
			}
		}
	}
	response = request(f.admin, "GET", "/api/v1/provisioning", "")
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "private-default") {
		t.Fatal("provisioning exposed hidden defaults", response.Code, response.Body)
	}
	input = server.ServerConfiguration
	input.Name = "Renamed server"
	expectAccess(t, regular, "PUT", endpoint, configJSON(t, input), http.StatusOK)
	input.Variables["INTERNAL_SETTING"] = "unauthorized-override"
	expectAccess(t, regular, "PUT", endpoint, configJSON(t, input), http.StatusForbidden)
	var applied model.Deployment
	f.runtime.configure = func(d model.Deployment) { applied = d }
	expectAccess(t, regular, "POST", endpoint+"/configuration", "{}", http.StatusOK)
	if applied.Configuration.Variables["INTERNAL_SETTING"] != "managed-private-value" || applied.Configuration.Variables["INTERNAL_LOCKED"] != "locked-default" {
		t.Fatal("hidden runtime values were lost", applied.Configuration.Variables)
	}
	ember.Description = "Updated definition"
	ember.Variables[1].Default = "new-private-default"
	ember.Variables[2].Default = "new-locked-default"
	data, err = json.Marshal(ember)
	if err != nil {
		t.Fatal(err)
	}
	expectAccess(t, f.admin, "PUT", "/api/v1/embers/"+ember.ID, string(data), http.StatusCreated)
	delete(input.Variables, "INTERNAL_SETTING")
	data, err = json.Marshal(struct {
		model.ServerConfiguration
		ExpectedEmberID string `json:"expected_ember_id"`
	}{input, ember.ID})
	if err != nil {
		t.Fatal(err)
	}
	expectAccess(t, f.admin, "POST", endpoint+"/ember", string(data), http.StatusOK)
	expectAccess(t, regular, "POST", endpoint+"/configuration", "{}", http.StatusOK)
	if applied.Configuration.Variables["INTERNAL_SETTING"] != "managed-private-value" || applied.Configuration.Variables["INTERNAL_LOCKED"] != "new-locked-default" {
		t.Fatal("Ember update lost hidden overrides or retained a locked default", applied.Configuration.Variables)
	}
}
