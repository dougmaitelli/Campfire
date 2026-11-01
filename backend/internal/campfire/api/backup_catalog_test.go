package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
	accessperm "campfire/internal/shared/permissions"
)

func TestBackupCatalogFiltersServerPermissions(t *testing.T) {
	f := newAccessFixture(t)
	ownerUser, owner := f.user("owner", database.UserRole)
	_, member := f.user("member", database.UserRole)
	_, outsider := f.user("outsider", database.UserRole)
	_, _, input := serverFixture(t, f.admin)
	input.MemoryMB, input.DiskMB = 512, 1024
	servers := make([]model.Server, 0, 2)

	for i := range 2 {
		input.Name = fmt.Sprintf("Server %d", i+1)
		input.Ports[0].HostPort = 25565 + i
		response := request(f.admin, "POST", "/api/v1/servers", configJSON(t, input))

		var server model.Server
		if response.Code != http.StatusCreated || json.Unmarshal(response.Body.Bytes(), &server) != nil {
			t.Fatal("create server", response.Code, response.Body)
		}

		servers = append(servers, server)
	}

	endpoint := "/api/v1/servers/" + servers[0].ID
	expectAccess(t, f.admin, "PUT", endpoint+"/owner", fmt.Sprintf(`{"user_id":%d}`, ownerUser.ID), http.StatusOK)
	expectAccess(t, owner, "POST", endpoint+"/members", fmt.Sprintf(`{"username":"member","role_id":%d}`, database.OperatorRole), http.StatusOK)

	f.runtime.files = func(q model.FileRequest) (model.FileResult, error) {
		return model.FileResult{ServerID: q.ServerID, Entries: []model.FileEntry{{Name: "snapshot.tar.gz", Kind: model.FileKindFile, Size: 4, ModifiedAt: time.Now()}}}, nil
	}
	for _, server := range servers {
		expectAccess(t, f.admin, "GET", "/api/v1/servers/"+server.ID+"/backups", "", http.StatusOK)
	}

	checkCatalog := func(handler http.Handler, serverIDs []string, canDownload bool) {
		t.Helper()

		response := request(handler, "GET", "/api/v1/backups", "")

		var catalog []struct {
			model.CatalogBackup
			CanDownload bool `json:"can_download"`
		}
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &catalog) != nil || catalog == nil || len(catalog) != len(serverIDs) {
			t.Fatal("unexpected catalog", response.Code, response.Body)
		}

		for _, entry := range catalog {
			if !slices.Contains(serverIDs, entry.ServerID) || entry.CanDownload != canDownload {
				t.Fatal("catalog ignored server permissions", entry)
			}
		}

		if response.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("permission-filtered catalog was cacheable")
		}
	}

	expectAccess(t, f.panel, "GET", "/api/v1/backups", "", http.StatusUnauthorized)
	checkCatalog(f.admin, []string{servers[0].ID, servers[1].ID}, true)
	checkCatalog(owner, []string{servers[0].ID}, true)
	checkCatalog(member, []string{servers[0].ID}, true)
	checkCatalog(outsider, nil, false)

	scopes, err := json.Marshal(accessperm.Expand([]model.PermissionID{accessperm.ServerBackupsRead}))
	if err != nil {
		t.Fatal(err)
	}

	_, readToken := createTestAPIToken(t, owner, string(scopes))
	checkCatalog(bearerAPI(f.panel, readToken), []string{servers[0].ID}, false)

	scopes, err = json.Marshal([]model.PermissionID{accessperm.ServerView})
	if err != nil {
		t.Fatal(err)
	}

	_, viewToken := createTestAPIToken(t, f.admin, string(scopes))
	checkCatalog(bearerAPI(f.panel, viewToken), nil, false)

	response := request(f.admin, "POST", "/api/v1/roles", fmt.Sprintf(`{"name":"Backup viewer","scope":"server","permissions":[%q]}`, accessperm.ServerBackupsRead))

	var role service.RoleView
	if response.Code != http.StatusCreated || json.Unmarshal(response.Body.Bytes(), &role) != nil {
		t.Fatal("create backup role", response.Code, response.Body)
	}

	expectAccess(t, owner, "POST", endpoint+"/members", fmt.Sprintf(`{"username":"member","role_id":%d}`, role.ID), http.StatusOK)
	checkCatalog(member, []string{servers[0].ID}, false)
	expectAccess(t, member, "GET", endpoint+"/backups/snapshot.tar.gz", "", http.StatusForbidden)
	expectAccess(t, f.admin, "PUT", fmt.Sprintf("/api/v1/roles/%d", role.ID), fmt.Sprintf(`{"name":"Backup viewer","scope":"server","permissions":[%q]}`, accessperm.ServerView), http.StatusOK)
	checkCatalog(member, nil, false)
	expectAccess(t, member, "GET", endpoint+"/backups", "", http.StatusForbidden)
}
