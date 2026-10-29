package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
	accessperm "campfire/internal/shared/permissions"
)

func TestServerFeaturePermissionsAndOwnerOnlyAccess(t *testing.T) {
	f := newAccessFixture(t)
	_, _, input := serverFixture(t, f.admin)
	created := request(f.admin, "POST", "/api/v1/servers", configJSON(t, input))

	var server model.Server
	if created.Code != http.StatusCreated || json.Unmarshal(created.Body.Bytes(), &server) != nil {
		t.Fatal(created.Code, created.Body)
	}

	endpoint := "/api/v1/servers/" + server.ID

	cases := []struct {
		name                                                                      string
		grants                                                                    []string
		console, writeConsole, readFiles, writeFiles, deleteFiles, backups, power int
	}{
		{"power", []string{accessperm.ServerOperate}, 200, 403, 403, 403, 403, 403, 202},
		{"logs", []string{accessperm.ServerConsoleRead}, 200, 403, 403, 403, 403, 403, 403},
		{"reader", []string{accessperm.ServerFilesRead}, 403, 403, 200, 403, 403, 403, 403},
		{"writer", []string{accessperm.ServerFilesWrite}, 403, 403, 200, 200, 403, 403, 403},
		{"backup-reader", []string{accessperm.ServerBackupsRead}, 403, 403, 403, 403, 403, 200, 403},
		{"restore", []string{accessperm.ServerBackupsRestore}, 200, 403, 200, 200, 403, 200, 202},
		{"all-grants", accessperm.All(), 200, 200, 200, 200, 200, 200, 202},
	}
	for _, check := range cases {
		t.Run(check.name, func(t *testing.T) {
			user, handler := f.user(check.name, database.UserRole)

			body, err := json.Marshal(map[string]any{"name": check.name, "scope": "server", "permissions": append([]string{accessperm.ServerView}, check.grants...)})
			if err != nil {
				t.Fatal(err)
			}

			response := request(f.admin, "POST", "/api/v1/roles", string(body))

			var role service.RoleView
			if response.Code != http.StatusCreated || json.Unmarshal(response.Body.Bytes(), &role) != nil {
				t.Fatal(response.Code, response.Body)
			}

			expectAccess(t, f.admin, "POST", endpoint+"/access/grants", fmt.Sprintf(`{"username":%q,"access_role_id":%d}`, user.Username, role.ID), http.StatusNoContent)
			expectAccess(t, handler, "GET", endpoint+"/console", "", check.console)
			expectAccess(t, handler, "POST", endpoint+"/console", `{"command":"status"}`, check.writeConsole)
			expectAccess(t, handler, "GET", endpoint+"/files", "", check.readFiles)
			expectAccess(t, handler, "POST", endpoint+"/files", `{"operation":"write","path":"config.txt","data":"dGVzdA==","revision":"absent"}`, check.writeFiles)
			expectAccess(t, handler, "POST", endpoint+"/files", `{"operation":"chmod","path":"config.txt","mode":384}`, check.writeFiles)
			expectAccess(t, handler, "POST", endpoint+"/files", `{"operation":"delete","path":"config.txt"}`, check.deleteFiles)
			expectAccess(t, handler, "GET", endpoint+"/backups", "", check.backups)
			expectAccess(t, handler, "POST", endpoint+"/actions", `{"action":"start"}`, check.power)
			expectAccess(t, handler, "GET", endpoint+"/access", "", http.StatusForbidden)
			response = request(handler, "GET", endpoint, "")

			var visible model.Server
			if json.Unmarshal(response.Body.Bytes(), &visible) != nil || visible.CanManageAccess {
				t.Fatal("custom role gained ownership authority", response.Body)
			}
		})
	}

	for _, invalid := range []string{accessperm.LegacyServerShare, "server.files.permissions", "server.backups.configure"} {
		body, err := json.Marshal(map[string]any{"name": "Invalid", "scope": "server", "permissions": []string{accessperm.ServerView, invalid}})
		if err != nil {
			t.Fatal(err)
		}

		expectAccess(t, f.admin, "POST", "/api/v1/roles", string(body), http.StatusBadRequest)
	}

	_, limited := createTestAPIToken(t, f.admin, `["server.view","server.configure"]`)
	expectAccess(t, bearerAPI(f.panel, limited), "GET", endpoint+"/access", "", http.StatusForbidden)
	expectAccess(t, f.admin, "GET", endpoint+"/access", "", http.StatusOK)
}

func TestWriteAndOperateDoNotImplicitlyGrantRestore(t *testing.T) {
	f := newAccessFixture(t)
	_, _, input := serverFixture(t, f.admin)
	response := request(f.admin, "POST", "/api/v1/servers", configJSON(t, input))

	var server model.Server
	if response.Code != http.StatusCreated || json.Unmarshal(response.Body.Bytes(), &server) != nil {
		t.Fatal(response.Code, response.Body)
	}

	_, scoped := createTestAPIToken(t, f.admin, `["server.view","server.operate","server.files.write","server.backups.read"]`)
	before := f.runtime.calls.Load()
	expectAccess(t, bearerAPI(f.panel, scoped), "POST", "/api/v1/servers/"+server.ID+"/actions", `{"action":"restore","backup":"snapshot.tar.gz"}`, http.StatusForbidden)

	if f.runtime.calls.Load() != before {
		t.Fatal("denied restore reached Spark")
	}
}
