package api_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/devfixture"
	"campfire/internal/shared/backuparchive"
	"campfire/internal/shared/model"
)

type delayedImportResponse struct{ http.ResponseWriter }

func (w delayedImportResponse) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w delayedImportResponse) WriteHeader(status int) {
	time.Sleep(100 * time.Millisecond)
	w.ResponseWriter.WriteHeader(status)
}

func TestBackupImportExtendsHTTPDeadlineAndRetainsPublicationReceipt(t *testing.T) {
	f := newAccessFixture(t)
	_, ember, configuration := serverFixture(t, f.admin)
	configuration.Variables = map[string]string{"SECONDS": "3600"}
	manifest := backuparchive.Manifest{Format: backuparchive.Format, BackupOptions: model.BackupOptions{Mode: model.BackupModeFull, Definition: model.NewBackupDefinition(model.Server{ServerConfiguration: configuration}, ember)}}

	metadata, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}

	archive, err := devfixture.Archive(map[string]string{model.BackupMetadataName: string(metadata)}, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	created := request(f.admin, "POST", "/api/v1/servers", configJSON(t, configuration))

	var server model.Server
	if created.Code != http.StatusCreated || json.Unmarshal(created.Body.Bytes(), &server) != nil {
		t.Fatal(created.Code, created.Body)
	}

	token := stagePortableBackup(t, f.admin, server.ID, archive)
	f.runtime.files = func(q model.FileRequest) (model.FileResult, error) {
		return model.FileResult{ServerID: q.ServerID, UploadID: "upload", Path: q.Path, Entries: []model.FileEntry{}}, nil
	}
	host := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.admin.ServeHTTP(delayedImportResponse{w}, r)
	}))
	host.Config.WriteTimeout = 25 * time.Millisecond
	host.Start()
	t.Cleanup(host.Close)

	for _, path := range []string{
		"/api/v1/backup-imports/" + token + "/inspect",
		"/api/v1/servers/" + server.ID + "/backup-imports/" + token,
		"/api/v1/servers/" + server.ID + "/backup-imports/" + token,
	} {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, host.URL+path, strings.NewReader("{}"))
		if err != nil {
			t.Fatal(err)
		}

		req.Header.Set("Content-Type", "application/json")

		response, err := host.Client().Do(req)
		if err != nil {
			t.Fatal("import inherited the short HTTP deadline", err)
		}

		body, readErr := io.ReadAll(response.Body)

		_ = response.Body.Close()
		if readErr != nil || response.StatusCode >= http.StatusBadRequest {
			t.Fatal(response.StatusCode, string(body), readErr)
		}
	}
}

func stagePortableBackup(t *testing.T, handler http.Handler, target string, data []byte) string {
	t.Helper()

	response := request(handler, "POST", "/api/v1/backup-imports", fmt.Sprintf(`{"server_id":%q,"size":%d}`, target, len(data)))

	var begun struct {
		Token string `json:"token"`
	}
	if response.Code != http.StatusCreated || json.Unmarshal(response.Body.Bytes(), &begun) != nil || begun.Token == "" {
		t.Fatal(response.Code, response.Body)
	}

	t.Cleanup(func() { request(handler, "DELETE", "/api/v1/backup-imports/"+begun.Token, "") })

	body, err := json.Marshal(struct {
		Offset int64  `json:"offset"`
		Data   []byte `json:"data"`
	}{Data: data})
	if err != nil {
		t.Fatal(err)
	}

	expectAccess(t, handler, "POST", "/api/v1/backup-imports/"+begun.Token+"/chunks", string(body), http.StatusOK)

	return begun.Token
}

func TestPortableBackupPreviewAndCreationPreserveHiddenValues(t *testing.T) {
	f := newAccessFixture(t)
	_, creator := f.user("backup-creator", database.CreatorRole)
	_, other := f.user("backup-other", database.CreatorRole)
	spark, ember, configuration := serverFixture(t, f.admin)
	ember.Variables = append(ember.Variables, model.Variable{Name: "TOKEN", Default: "default-token", Hidden: true, Editable: true})

	encoded, err := json.Marshal(ember)
	if err != nil {
		t.Fatal(err)
	}

	expectAccess(t, f.admin, "PUT", "/api/v1/embers/"+ember.ID, string(encoded), http.StatusCreated)
	expectAccess(t, f.admin, "PUT", "/api/v1/sparks/"+spark.ID+"/roles", `{"role_ids":[2]}`, http.StatusOK)

	configuration.Variables = map[string]string{"SECONDS": "3600", "TOKEN": "preserved-secret"}
	manifest := backuparchive.Manifest{Format: backuparchive.Format, BackupOptions: model.BackupOptions{Mode: model.BackupModeFull, Definition: model.NewBackupDefinition(model.Server{ServerConfiguration: configuration}, ember)}}

	metadata, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}

	archive, err := devfixture.Archive(map[string]string{model.BackupMetadataName: string(metadata), "world.dat": "saved world"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	token := stagePortableBackup(t, f.admin, "", archive)
	expectAccess(t, other, "POST", "/api/v1/backup-imports/"+token+"/inspect", `{}`, http.StatusNotFound)

	response := request(f.admin, "POST", "/api/v1/backup-imports/"+token+"/inspect", `{}`)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "preserved-secret") || strings.Contains(response.Body.String(), "default-token") || strings.Contains(response.Body.String(), `"TOKEN"`) {
		t.Fatal("preview failed or exposed hidden values", response.Code, response.Body)
	}

	configuration.Variables = map[string]string{}

	input, err := json.Marshal(struct {
		model.ServerConfiguration
		BackupToken string `json:"backup_token"`
	}{configuration, token})
	if err != nil {
		t.Fatal(err)
	}

	response = request(f.admin, "POST", "/api/v1/servers", string(input))

	var server model.Server
	if response.Code != http.StatusCreated || json.Unmarshal(response.Body.Bytes(), &server) != nil {
		t.Fatal(response.Code, response.Body)
	}

	if strings.Contains(response.Body.String(), "preserved-secret") {
		t.Fatal("server response exposed hidden values")
	}

	f.runtime.configure = func(deployment model.Deployment) {
		if deployment.Configuration.Variables["TOKEN"] != "preserved-secret" {
			t.Error("hidden value not restored")
		}
	}

	expectAccess(t, f.admin, "POST", "/api/v1/servers/"+server.ID+"/configuration", `{}`, http.StatusOK)
	response = request(f.admin, "POST", "/api/v1/servers", string(input))

	var repeated model.Server
	if response.Code != http.StatusCreated || json.Unmarshal(response.Body.Bytes(), &repeated) != nil || repeated.ID != server.ID {
		t.Fatal("retry created a duplicate", response.Code, response.Body)
	}

	expectAccess(t, creator, "POST", "/api/v1/backup-imports", `{"server_id":"unassigned","size":100}`, http.StatusForbidden)
	forged := stagePortableBackup(t, creator, "", archive)
	expectAccess(t, creator, "POST", "/api/v1/backup-imports/"+forged+"/inspect", `{}`, http.StatusBadRequest)
	expectAccess(t, creator, "POST", "/api/v1/servers", strings.Replace(string(input), token, forged, 1), http.StatusForbidden)
	expectAccess(t, creator, "DELETE", "/api/v1/backup-imports/"+forged, "", http.StatusNoContent)

	manifest.Definition.Configuration.Variables["TOKEN"] = "default-token"

	metadata, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}

	defaultArchive, err := devfixture.Archive(map[string]string{model.BackupMetadataName: string(metadata)}, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	approved := stagePortableBackup(t, creator, "", defaultArchive)
	expectAccess(t, creator, "POST", "/api/v1/backup-imports/"+approved+"/inspect", `{}`, http.StatusOK)

	configuration.Name = "Creator restored server"
	configuration.Ports[0].HostPort = 25566

	creatorInput, err := json.Marshal(struct {
		model.ServerConfiguration
		BackupToken string `json:"backup_token"`
	}{configuration, approved})
	if err != nil {
		t.Fatal(err)
	}

	expectAccess(t, creator, "POST", "/api/v1/servers", string(creatorInput), http.StatusCreated)

	manifest.Definition.Ember.Name = "Unknown definition"

	metadata, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}

	archive, err = devfixture.Archive(map[string]string{model.BackupMetadataName: string(metadata)}, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	unknown := stagePortableBackup(t, creator, "", archive)
	expectAccess(t, creator, "POST", "/api/v1/backup-imports/"+unknown+"/inspect", `{}`, http.StatusBadRequest)
}
