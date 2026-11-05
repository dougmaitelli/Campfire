package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"
	accessperm "campfire/internal/shared/permissions"
)

func TestCopyKeepsHiddenVariablesPrivateAndUsesCreationOwnership(t *testing.T) {
	f := newAccessFixture(t)
	creator, regular := f.user("copy-creator", database.CreatorRole)
	_, outsider := f.user("copy-outsider", database.CreatorRole)
	spark, ember, input := serverFixture(t, f.admin)
	ember.Variables = append(ember.Variables, model.Variable{Name: "PRIVATE_TOKEN", Default: "private-default", Hidden: true, Editable: true})

	encoded, err := json.Marshal(ember)
	if err != nil {
		t.Fatal(err)
	}

	expectAccess(t, f.admin, "PUT", "/api/v1/embers/"+ember.ID, string(encoded), http.StatusCreated)
	expectAccess(t, f.admin, "PUT", "/api/v1/sparks/"+spark.ID+"/roles", `{"role_ids":[2]}`, http.StatusOK)

	input.Variables = map[string]string{"PRIVATE_TOKEN": "retained-private-value"}

	encoded, err = json.Marshal(struct {
		model.ServerConfiguration
		OwnerUserID int64 `json:"owner_user_id"`
	}{input, creator.ID})
	if err != nil {
		t.Fatal(err)
	}

	response := request(f.admin, "POST", "/api/v1/servers", string(encoded))

	var source model.Server
	if response.Code != http.StatusCreated || json.Unmarshal(response.Body.Bytes(), &source) != nil {
		t.Fatal(response.Code, response.Body)
	}

	path := "/api/v1/servers/" + source.ID

	response = request(regular, "GET", path+"/copy", "")
	if response.Code != http.StatusOK {
		t.Fatal(response.Code, response.Body)
	}

	for _, secret := range []string{"PRIVATE_TOKEN", "private-default", "retained-private-value"} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatal("copy template exposed hidden values", secret)
		}
	}

	input = source.ServerConfiguration
	input.Name = "Private settings copy"
	input.Ports = nil

	encoded, err = json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}

	expectAccess(t, outsider, "POST", path+"/copy", string(encoded), http.StatusNotFound)
	response = request(regular, "POST", path+"/copy", string(encoded))

	var copied model.Server
	if response.Code != http.StatusCreated || json.Unmarshal(response.Body.Bytes(), &copied) != nil || copied.ID == source.ID || copied.OwnerUserID != creator.ID {
		t.Fatal("copy creation failed", response.Code, response.Body)
	}

	if strings.Contains(response.Body.String(), "PRIVATE_TOKEN") || strings.Contains(response.Body.String(), "retained-private-value") {
		t.Fatal("copy response exposed hidden values", response.Body)
	}

	var applied model.Deployment

	f.runtime.configure = func(deployment model.Deployment) { applied = deployment }

	expectAccess(t, regular, "POST", "/api/v1/servers/"+copied.ID+"/configuration", `{}`, http.StatusOK)

	if applied.Configuration.Variables["PRIVATE_TOKEN"] != "retained-private-value" {
		t.Fatal("hidden override was lost")
	}

	input.Name = "Forged private copy"
	input.Variables["PRIVATE_TOKEN"] = "forged"
	expectAccess(t, regular, "POST", path+"/copy", configJSON(t, input), http.StatusBadRequest)
	expectAccess(t, regular, "POST", path+"/quick-copy", fmt.Sprintf(`{"spark_id":%q,"owner_user_id":1}`, spark.ID), http.StatusBadRequest)
	// Creating a new server through Copy cannot select someone else's ownership.
	input.Variables = map[string]string{}

	encoded, err = json.Marshal(struct {
		model.ServerConfiguration
		OwnerUserID int64 `json:"owner_user_id"`
	}{input, 1})
	if err != nil {
		t.Fatal(err)
	}

	expectAccess(t, regular, "POST", path+"/copy", string(encoded), http.StatusForbidden)
}

func TestCopyRequiresProvisioningAndSourceConfiguration(t *testing.T) {
	f := newAccessFixture(t)
	owner, regular := f.user("copy-owner-no-create", database.UserRole)
	spark, _, input := serverFixture(t, f.admin)

	encoded, err := json.Marshal(struct {
		model.ServerConfiguration
		OwnerUserID int64 `json:"owner_user_id"`
	}{input, owner.ID})
	if err != nil {
		t.Fatal(err)
	}

	response := request(f.admin, "POST", "/api/v1/servers", string(encoded))

	var source model.Server
	if response.Code != http.StatusCreated || json.Unmarshal(response.Body.Bytes(), &source) != nil {
		t.Fatal(response.Code, response.Body)
	}

	path := "/api/v1/servers/" + source.ID
	input.Name, input.Ports = "Denied copy", nil

	expectAccess(t, regular, "GET", path+"/copy", "", http.StatusForbidden)
	expectAccess(t, regular, "POST", path+"/copy", configJSON(t, input), http.StatusForbidden)

	_, creator := f.user("copy-no-source", database.CreatorRole)
	expectAccess(t, creator, "POST", path+"/quick-copy", fmt.Sprintf(`{"spark_id":%q}`, spark.ID), http.StatusNotFound)
	// An Admin can deliberately select an owner on the reviewed Copy path.
	input.Name = "Admin-owned copy"

	encoded, err = json.Marshal(struct {
		model.ServerConfiguration
		OwnerUserID int64 `json:"owner_user_id"`
	}{input, owner.ID})
	if err != nil {
		t.Fatal(err)
	}

	response = request(f.admin, "POST", path+"/copy", string(encoded))

	var copied model.Server
	if response.Code != http.StatusCreated || json.Unmarshal(response.Body.Bytes(), &copied) != nil || copied.OwnerUserID != owner.ID {
		t.Fatal(response.Code, response.Body)
	}
}

func TestCopyFilesRequiresSourceFileReadPermission(t *testing.T) {
	f := newAccessFixture(t)
	_, _, input := serverFixture(t, f.admin)
	created := request(f.admin, "POST", "/api/v1/servers", configJSON(t, input))

	var source model.Server
	if created.Code != http.StatusCreated || json.Unmarshal(created.Body.Bytes(), &source) != nil {
		t.Fatal(created.Code, created.Body)
	}

	_, token := createTestAPIToken(t, f.admin, fmt.Sprintf(`[%q,%q,%q]`, accessperm.ServersCreate, accessperm.ServerView, accessperm.ServerConfigure))
	handler := bearerAPI(f.panel, token)
	input.Name, input.Ports = "Read-protected source copy", nil

	body, err := json.Marshal(struct {
		model.ServerConfiguration
		CopyFiles bool `json:"copy_files"`
	}{input, true})
	if err != nil {
		t.Fatal(err)
	}

	path := "/api/v1/servers/" + source.ID
	expectAccess(t, handler, "POST", path+"/copy", string(body), http.StatusForbidden)
	expectAccess(t, handler, "POST", path+"/quick-copy", fmt.Sprintf(`{"spark_id":%q}`, input.SparkID), http.StatusForbidden)
	expectAccess(t, handler, "POST", path+"/copy", configJSON(t, input), http.StatusCreated)

	response := request(f.admin, "POST", path+"/quick-copy", fmt.Sprintf(`{"spark_id":%q}`, input.SparkID))
	if response.Code != http.StatusConflict || strings.Contains(strings.ToLower(response.Body.String()), "stop") {
		t.Fatal("uninstalled source rejection incorrectly required stopping", response.Code, response.Body)
	}
}
