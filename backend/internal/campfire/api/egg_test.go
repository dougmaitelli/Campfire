package api_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
)

func TestEggPreviewImportPersistenceAndServerValidation(t *testing.T) {
	handler, _ := openAPI(t, filepath.Join(t.TempDir(), "panel.db"))
	_, _, input := serverFixture(t, handler)

	data, err := os.ReadFile("../service/testdata/eggs/ptdl-v2.json")
	if err != nil {
		t.Fatal(err)
	}

	body := func(source []byte, ack bool) string {
		t.Helper()

		encoded, err := json.Marshal(map[string]any{"definition": string(source), "acknowledge_warnings": ack})
		if err != nil {
			t.Fatal(err)
		}

		return string(encoded)
	}
	response := request(handler, "POST", "/api/v1/embers/preview", body(data, false))

	var preview service.EmberPreview

	if response.Code != http.StatusOK {
		t.Fatal(response.Code, response.Body)
	}

	if err := json.Unmarshal(response.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}

	if preview.Ember == nil || len(preview.Errors) > 0 || len(preview.Warnings) == 0 || preview.Format != "PTDL_v2" {
		t.Fatal("invalid compatibility preview", response.Body)
	}

	count := func() int {
		t.Helper()

		response := request(handler, "GET", "/api/v1/embers", "")

		var items []model.Ember
		if err := json.Unmarshal(response.Body.Bytes(), &items); err != nil {
			t.Fatal(err)
		}

		return len(items)
	}
	if count() != 1 {
		t.Fatal("preview persisted an Ember")
	}

	expectAccess(t, handler, "POST", "/api/v1/embers", body(data, false), http.StatusBadRequest)

	response = request(handler, "POST", "/api/v1/embers", body(data, true))
	if response.Code != http.StatusCreated {
		t.Fatal(response.Code, response.Body)
	}

	var imported model.Ember
	if err := json.Unmarshal(response.Body.Bytes(), &imported); err != nil {
		t.Fatal(err)
	}

	if imported.ID == "" || imported.Schema != 1 || imported.Origin == nil || imported.Origin.Author != "fixtures@example.invalid" || count() != 2 {
		t.Fatal("converted Ember not persisted", response.Body)
	}

	expectAccess(t, handler, "GET", "/api/v1/embers/"+imported.ID, "", http.StatusOK)
	// An acknowledgement must never authorize features added after the preview.
	changed := strings.Replace(string(data), `"file_denylist": []`, `"file_denylist": ["secret"]`, 1)
	expectAccess(t, handler, "POST", "/api/v1/embers", body([]byte(changed), true), http.StatusBadRequest)

	if count() != 2 {
		t.Fatal("rejected import persisted")
	}

	input.EmberID = imported.ID
	input.Image = imported.Images["Java 21"]
	input.Variables = map[string]string{"SERVER_JARFILE": "server.jar", "MAX_PLAYERS": "129"}
	expectAccess(t, handler, "POST", "/api/v1/servers", configJSON(t, input), http.StatusBadRequest)
	input.Variables["MAX_PLAYERS"] = "20"
	expectAccess(t, handler, "POST", "/api/v1/servers", configJSON(t, input), http.StatusCreated)
}

func TestEggURLRuleImportAndServerValidation(t *testing.T) {
	handler, _ := openAPI(t, filepath.Join(t.TempDir(), "panel.db"))
	_, _, input := serverFixture(t, handler)
	definition := `{"meta":{"version":"PLCN_v3"},"name":"URL validation","tags":{},"docker_images":{"Alpine":"alpine:3.22"},"startup_commands":{"Default":"sleep 3600"},"variables":[{"env_variable":"DOWNLOAD_URL","default_value":"https://example.com/file","user_viewable":true,"user_editable":true,"rules":["required","url"]}]}`

	body, err := json.Marshal(map[string]any{"definition": definition, "acknowledge_warnings": true})
	if err != nil {
		t.Fatal(err)
	}

	response := request(handler, "POST", "/api/v1/embers", string(body))

	var imported model.Ember
	if response.Code != http.StatusCreated || json.Unmarshal(response.Body.Bytes(), &imported) != nil {
		t.Fatal(response.Code, response.Body)
	}

	response = request(handler, "GET", "/api/v1/embers/"+imported.ID, "")
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &imported) != nil || len(imported.Variables) != 1 || imported.Variables[0].Validation == nil || imported.Variables[0].Validation.Format != "url" {
		t.Fatal("URL constraint was lost in storage", response.Code, response.Body)
	}

	input.EmberID = imported.ID
	input.Image = imported.Images["Alpine"]
	input.Variables = map[string]string{"DOWNLOAD_URL": "relative/file"}
	expectAccess(t, handler, "POST", "/api/v1/servers", configJSON(t, input), http.StatusBadRequest)
	input.Variables["DOWNLOAD_URL"] = "https://example.com/another-file"
	expectAccess(t, handler, "POST", "/api/v1/servers", configJSON(t, input), http.StatusCreated)
}
