package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"campfire/internal/shared/model"
	accessperm "campfire/internal/shared/permissions"
)

func TestBackupMetadataUsesCreationPermissionAndStableArchiveIdentity(t *testing.T) {
	f := newAccessFixture(t)
	_, _, input := serverFixture(t, f.admin)
	created := request(f.admin, "POST", "/api/v1/servers", configJSON(t, input))

	var server model.Server
	if created.Code != http.StatusCreated || json.Unmarshal(created.Body.Bytes(), &server) != nil {
		t.Fatal(created.Code, created.Body)
	}

	f.runtime.files = func(q model.FileRequest) (model.FileResult, error) {
		return model.FileResult{ServerID: q.ServerID, Entries: []model.FileEntry{{Name: "snapshot.tar.gz", Size: 4, ModifiedAt: time.Now()}}}, nil
	}
	path := "/api/v1/servers/" + server.ID + "/backups"
	expectAccess(t, f.admin, "GET", path, "", http.StatusOK)

	readScopes, err := json.Marshal(accessperm.Expand([]model.PermissionID{accessperm.ServerBackupsRead}))
	if err != nil {
		t.Fatal(err)
	}

	_, readSecret := createTestAPIToken(t, f.admin, string(readScopes))
	expectAccess(t, bearerAPI(f.panel, readSecret), "PATCH", path+"/snapshot.tar.gz", `{"pinned":true}`, http.StatusForbidden)

	writeScopes, err := json.Marshal(accessperm.Expand([]model.PermissionID{accessperm.ServerBackupsCreate}))
	if err != nil {
		t.Fatal(err)
	}

	_, writeSecret := createTestAPIToken(t, f.admin, string(writeScopes))
	editor := bearerAPI(f.panel, writeSecret)
	response := request(editor, "PATCH", path+"/snapshot.tar.gz", `{"display_name":"  Before update  ","pinned":true}`)

	var backup model.Backup
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &backup) != nil || backup.Name != "snapshot.tar.gz" || backup.DisplayName != "Before update" || !backup.Pinned {
		t.Fatal("metadata update changed archive identity or lost fields", response.Code, response.Body)
	}

	expectAccess(t, f.admin, "DELETE", path+"/snapshot.tar.gz", "", http.StatusConflict)
	expectAccess(t, editor, "PATCH", path+"/missing.tar.gz", `{"pinned":true}`, http.StatusNotFound)

	for _, body := range []string{`{}`, `{"name":"renamed.tar.gz"}`, `{"pinned":"true"}`, `{"display_name":""}`, `{"display_name":"a\nb"}`, fmt.Sprintf(`{"display_name":%q}`, strings.Repeat("a", 101))} {
		expectAccess(t, editor, "PATCH", path+"/snapshot.tar.gz", body, http.StatusBadRequest)
	}

	response = request(editor, "GET", "/api/v1/backups", "")

	var catalog []struct {
		model.CatalogBackup
		CanEdit bool `json:"can_edit"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &catalog) != nil || len(catalog) != 1 || !catalog[0].CanEdit || !catalog[0].Pinned || catalog[0].DisplayName != backup.DisplayName {
		t.Fatal("catalog did not retain metadata or permission", response.Code, response.Body)
	}

	response = request(bearerAPI(f.panel, readSecret), "GET", "/api/v1/backups", "")
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &catalog) != nil || len(catalog) != 1 || catalog[0].CanEdit {
		t.Fatal("read-only catalog allowed editing", response.Code, response.Body)
	}

	expectAccess(t, editor, "PATCH", path+"/snapshot.tar.gz", `{"pinned":false}`, http.StatusOK)
	expectAccess(t, f.admin, "DELETE", path+"/snapshot.tar.gz", "", http.StatusAccepted)
}
