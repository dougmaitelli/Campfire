package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"campfire/internal/campfire/api"
	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
)

type accessRuntime struct {
	calls   atomic.Int64
	files   func(model.FileRequest) (model.FileResult, error)
	metrics func(model.RuntimeRequest) (model.RuntimeStatus, error)
	console func(context.Context, string, func(model.ConsoleEvent) error) error
}

func (d *accessRuntime) Console(ctx context.Context, _ string, id string, emit func(model.ConsoleEvent) error) error {
	d.calls.Add(1)

	if d.console != nil {
		return d.console(ctx, id, emit)
	}

	return nil
}

func (d *accessRuntime) Deploy(_ context.Context, _ string, definition model.Deployment) (model.ConfigurationReceipt, error) {
	d.calls.Add(1)
	return model.ConfigurationReceipt{ServerID: definition.ServerID, Revision: strings.Repeat("a", 64)}, nil
}
func (d *accessRuntime) Action(_ context.Context, _ string, action model.RuntimeRequest) (model.RuntimeStatus, error) {
	d.calls.Add(1)

	if action.Action == "metrics" && d.metrics != nil {
		return d.metrics(action)
	}

	return model.RuntimeStatus{ServerID: action.ServerID, State: "not_installed"}, nil
}
func (d *accessRuntime) Files(_ context.Context, _ string, input model.FileRequest) (model.FileResult, error) {
	d.calls.Add(1)

	if d.files != nil {
		return d.files(input)
	}

	return model.FileResult{ServerID: input.ServerID, Path: input.Path, Entries: []model.FileEntry{}}, nil
}

type accessFixture struct {
	t       *testing.T
	auth    *service.AuthService
	admin   http.Handler
	panel   http.Handler
	runtime *accessRuntime
}

func newAccessFixture(t *testing.T) *accessFixture {
	t.Helper()

	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	sparks := service.NewSparkService(repository.NewSparkRepository(db.ORM()), nil)
	embers := service.NewEmberService(repository.NewEmberRepository(db.ORM()))
	servers := service.NewServerService(repository.NewServerRepository(db.ORM()), embers)
	dispatch := &accessRuntime{}
	deployment := &service.DeploymentService{Servers: servers, Sparks: sparks, Embers: embers, Remote: dispatch}
	deployment.Backups = service.NewBackupService(repository.NewBackupRepository(db.ORM()), deployment, nil)
	panel := api.New(sparks, embers, servers).WithDeployment(deployment)
	admin := authenticatedTestAPI(t, db, panel)

	return &accessFixture{t, service.NewAuthService(repository.NewAuthRepository(db.ORM())), admin, panel.Handler(), dispatch}
}

func (f *accessFixture) user(name string, role int64) (database.User, http.Handler) {
	f.t.Helper()

	response := request(f.admin, "POST", "/api/v1/users", fmt.Sprintf(`{"username":%q,"password":%q,"role_id":%d}`, name, testAdminPassword, role))
	if response.Code != http.StatusCreated {
		f.t.Fatal(response.Code, response.Body)
	}

	if strings.Contains(response.Body.String(), "password_hash") || strings.Contains(response.Body.String(), "pbkdf2") {
		f.t.Fatal("credential material exposed")
	}

	var user database.User
	if err := json.Unmarshal(response.Body.Bytes(), &user); err != nil {
		f.t.Fatal(err)
	}

	session, token, err := f.auth.Login(context.Background(), name, testAdminPassword, "")
	if err != nil {
		f.t.Fatal(err)
	}

	return user, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.AddCookie(&http.Cookie{Name: "campfire_session", Value: token})
		r.Header.Set("X-CSRF-Token", session.CSRF)
		f.panel.ServeHTTP(w, r)
	})
}

func expectAccess(t *testing.T, handler http.Handler, method, path, body string, code int) {
	t.Helper()

	response := request(handler, method, path, body)
	if response.Code != code {
		t.Fatalf("%s %s: got %d, want %d: %s", method, path, response.Code, code, response.Body)
	}
}

func TestUserDisplayNames(t *testing.T) {
	f := newAccessFixture(t)
	user, regular := f.user("named-user", database.UserRole)
	path := fmt.Sprintf("/api/v1/users/%d", user.ID)

	for _, name := range []string{"  Jane Doe  ", ""} {
		body := fmt.Sprintf(`{"name":%q,"role_id":%d}`, name, database.UserRole)
		expectAccess(t, f.admin, "PUT", path, body, http.StatusOK)

		response := request(regular, "GET", "/api/v1/auth/session", "")
		if response.Code != http.StatusOK {
			t.Fatal("name edit revoked the session", response.Code)
		}

		var state struct {
			Name     string `json:"name"`
			Username string `json:"username"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &state); err != nil {
			t.Fatal(err)
		}

		if state.Name != strings.TrimSpace(name) || state.Username != user.Username {
			t.Fatal("session did not expose the updated name", state)
		}
	}

	for _, name := range []string{strings.Repeat("x", 101), "bad\nname"} {
		body := fmt.Sprintf(`{"name":%q,"role_id":%d}`, name, database.UserRole)
		expectAccess(t, f.admin, "PUT", path, body, http.StatusBadRequest)
	}

	body := fmt.Sprintf(`{"username":"with-name","name":"John Smith","password":%q,"role_id":%d}`, testAdminPassword, database.UserRole)

	response := request(f.admin, "POST", "/api/v1/users", body)
	if response.Code != http.StatusCreated {
		t.Fatal(response.Code, response.Body)
	}

	var created database.User
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil || created.Name != "John Smith" {
		t.Fatal("name not persisted on creation", created, err)
	}
}

func TestSparkTransferRequiresPlatformPermissions(t *testing.T) {
	fixture := newAccessFixture(t)

	_, regular := fixture.user("regular", 3)
	for _, route := range []struct{ method, path string }{
		{"GET", "/api/v1/servers/example/transfers"},
		{"POST", "/api/v1/servers/example/transfers"},
		{"POST", "/api/v1/transfers/example/retry"},
		{"POST", "/api/v1/transfers/example/cancel"},
		{"DELETE", "/api/v1/sparks/example"},
	} {
		expectAccess(t, regular, route.method, route.path, "{}", http.StatusForbidden)
	}

	expectAccess(t, fixture.admin, "GET", "/api/v1/servers/example/transfers", "", http.StatusServiceUnavailable)
}

func TestServerAuthorizationOwnershipAndRevocation(t *testing.T) {
	f := newAccessFixture(t)
	op, operator := f.user("operator", database.UserRole)
	creator, owner := f.user("creator", database.CreatorRole)
	node, _, input := serverFixture(t, f.admin)
	input.Name = "Owned game"

	forged, err := json.Marshal(struct {
		model.ServerConfiguration
		OwnerUserID int64 `json:"owner_user_id"`
	}{input, 1})
	if err != nil {
		t.Fatal(err)
	}

	expectAccess(t, owner, "POST", "/api/v1/servers", string(forged), http.StatusForbidden)
	expectAccess(t, owner, "POST", "/api/v1/servers", configJSON(t, input), http.StatusForbidden)
	expectAccess(t, f.admin, "PUT", "/api/v1/sparks/"+node.ID+"/roles", `{"role_ids":[2]}`, http.StatusOK)

	response := request(owner, "POST", "/api/v1/servers", configJSON(t, input))
	if response.Code != http.StatusCreated {
		t.Fatal(response.Code, response.Body)
	}

	var server model.Server
	if err := json.Unmarshal(response.Body.Bytes(), &server); err != nil {
		t.Fatal(err)
	}

	if server.OwnerUserID != creator.ID {
		t.Fatal("creator did not become the owner")
	}

	endpoint := "/api/v1/servers/" + server.ID
	blocked := []struct{ method, suffix, body string }{
		{"GET", "", ""}, {"PUT", "", configJSON(t, input)}, {"DELETE", "", ""},
		{"POST", "/configuration", "{}"}, {"POST", "/actions", `{"action":"start"}`}, {"GET", "/status", ""},
		{"GET", "/console", ""}, {"POST", "/console", `{"command":"secret"}`},
		{"GET", "/console/stream", ""},
		{"GET", "/files", ""}, {"POST", "/files", `{"operation":"delete","path":"world"}`},
		{"GET", "/backups", ""}, {"GET", "/backups/backup.tar.gz", ""},
		{"GET", "/editor", ""}, {"GET", "/access", ""}, {"POST", "/members", `{"username":"admin"}`},
		{"DELETE", "/members/1", ""},
	}
	before := f.runtime.calls.Load()

	for _, check := range blocked {
		expectAccess(t, operator, check.method, endpoint+check.suffix, check.body, http.StatusNotFound)
	}

	if f.runtime.calls.Load() != before {
		t.Fatal("denied requests reached the runtime")
	}

	if response := request(operator, "GET", "/api/v1/servers", ""); response.Body.String() != "[]\n" {
		t.Fatal("unrelated server leaked", response.Body)
	}

	expectAccess(t, owner, "POST", endpoint+"/members", `{"username":"operator","role_id":1}`, http.StatusForbidden)
	expectAccess(t, owner, "POST", endpoint+"/members", `{"username":"operator","role_id":5}`, http.StatusOK)
	expectAccess(t, operator, "GET", endpoint, "", http.StatusOK)
	expectAccess(t, operator, "POST", endpoint+"/actions", `{"action":"start"}`, http.StatusAccepted)
	expectAccess(t, operator, "GET", endpoint+"/files", "", http.StatusOK)

	for _, check := range []struct{ method, suffix string }{{"PUT", ""}, {"DELETE", ""}, {"GET", "/editor"}, {"GET", "/access"}, {"POST", "/members"}} {
		expectAccess(t, operator, check.method, endpoint+check.suffix, "{}", http.StatusForbidden)
	}

	input.Description = "Owner edit"
	expectAccess(t, owner, "PUT", endpoint, configJSON(t, input), http.StatusOK)
	expectAccess(t, owner, "GET", endpoint+"/editor", "", http.StatusOK)
	expectAccess(t, f.admin, "PUT", "/api/v1/sparks/"+node.ID+"/roles", `{"role_ids":[]}`, http.StatusOK)
	expectAccess(t, owner, "PUT", endpoint, configJSON(t, input), http.StatusOK)
	expectAccess(t, owner, "DELETE", endpoint+"/members/"+fmt.Sprint(op.ID), "", http.StatusNoContent)

	for _, check := range blocked {
		expectAccess(t, operator, check.method, endpoint+check.suffix, check.body, http.StatusNotFound)
	}

	expectAccess(t, owner, "PUT", endpoint+"/owner", fmt.Sprintf(`{"user_id":%d}`, op.ID), http.StatusForbidden)
	expectAccess(t, f.admin, "PUT", endpoint+"/owner", fmt.Sprintf(`{"user_id":%d}`, op.ID), http.StatusOK)
	expectAccess(t, owner, "GET", endpoint, "", http.StatusNotFound)
	expectAccess(t, operator, "PUT", endpoint, configJSON(t, input), http.StatusOK)
	expectAccess(t, f.admin, "DELETE", "/api/v1/users/"+fmt.Sprint(op.ID), "", http.StatusConflict)
	expectAccess(t, f.admin, "PUT", "/api/v1/users/"+fmt.Sprint(op.ID), `{"role_id":3,"disabled":true}`, http.StatusOK)
	expectAccess(t, operator, "GET", endpoint, "", http.StatusUnauthorized)
	expectAccess(t, f.admin, "GET", endpoint, "", http.StatusOK)
}

func TestPlatformRoleBoundariesAndLiveServerRoleChanges(t *testing.T) {
	f := newAccessFixture(t)

	expectAccess(t, f.admin, "PUT", "/api/v1/users/1", `{"role_id":1,"disabled":false}`, http.StatusOK)
	expectAccess(t, f.admin, "GET", "/api/v1/users", "", http.StatusOK)

	user, handler := f.user("member", database.UserRole)
	for _, check := range []struct{ method, path string }{
		{"GET", "/sparks"}, {"POST", "/sparks"}, {"GET", "/sparks/node"}, {"PATCH", "/sparks/node"}, {"POST", "/sparks/node/enrollment"}, {"GET", "/sparks/node/roles"}, {"PUT", "/sparks/node/roles"},
		{"GET", "/users"}, {"POST", "/users"}, {"PUT", "/users/1"}, {"DELETE", "/users/1"},
		{"GET", "/roles"}, {"POST", "/roles"}, {"PUT", "/roles/1"}, {"DELETE", "/roles/1"},
		{"GET", "/embers"}, {"POST", "/embers"}, {"POST", "/embers/preview"}, {"POST", "/embers/preview-url"}, {"GET", "/provisioning"}, {"POST", "/servers"},
	} {
		expectAccess(t, handler, check.method, "/api/v1"+check.path, "{}", http.StatusForbidden)
	}

	expectAccess(t, f.admin, "PUT", "/api/v1/users/1", `{"role_id":3}`, http.StatusConflict)
	expectAccess(t, f.admin, "PUT", "/api/v1/users/1", `{"role_id":1,"disabled":true}`, http.StatusConflict)
	expectAccess(t, f.admin, "DELETE", "/api/v1/users/1", "", http.StatusConflict)
	expectAccess(t, f.admin, "PUT", "/api/v1/roles/1", `{"name":"Fake admin","scope":"platform","permissions":[]}`, http.StatusForbidden)
	expectAccess(t, f.admin, "POST", "/api/v1/roles", `{"name":"Escalation","scope":"platform","permissions":["sparks.manage"]}`, http.StatusBadRequest)
	_, _, input := serverFixture(t, f.admin)
	response := request(f.admin, "POST", "/api/v1/servers", configJSON(t, input))

	var server model.Server
	if err := json.Unmarshal(response.Body.Bytes(), &server); err != nil {
		t.Fatal(err)
	}

	response = request(f.admin, "POST", "/api/v1/roles", `{"name":"Helper","scope":"server","permissions":["server.view","server.operate"]}`)
	if response.Code != http.StatusCreated {
		t.Fatal(response.Code, response.Body)
	}

	var role service.RoleView
	if err := json.Unmarshal(response.Body.Bytes(), &role); err != nil {
		t.Fatal(err)
	}

	endpoint := "/api/v1/servers/" + server.ID
	expectAccess(t, f.admin, "POST", endpoint+"/members", fmt.Sprintf(`{"username":"member","role_id":%d}`, role.ID), http.StatusOK)
	expectAccess(t, handler, "POST", endpoint+"/actions", `{"action":"start"}`, http.StatusAccepted)
	expectAccess(t, f.admin, "PUT", "/api/v1/roles/"+fmt.Sprint(role.ID), `{"name":"Helper","scope":"server","permissions":["server.view"]}`, http.StatusOK)
	expectAccess(t, handler, "GET", endpoint, "", http.StatusOK)
	expectAccess(t, handler, "POST", endpoint+"/actions", `{"action":"start"}`, http.StatusForbidden)
	expectAccess(t, f.admin, "DELETE", "/api/v1/roles/"+fmt.Sprint(role.ID), "", http.StatusConflict)
	expectAccess(t, f.admin, "DELETE", endpoint+"/members/"+fmt.Sprint(user.ID), "", http.StatusNoContent)
	expectAccess(t, handler, "GET", endpoint, "", http.StatusNotFound)
	expectAccess(t, f.admin, "DELETE", "/api/v1/roles/"+fmt.Sprint(role.ID), "", http.StatusNoContent)
}

func TestBackupDownloadStopsAfterServerAccessRevocation(t *testing.T) {
	f := newAccessFixture(t)
	user, handler := f.user("downloader", database.UserRole)
	_, _, input := serverFixture(t, f.admin)
	response := request(f.admin, "POST", "/api/v1/servers", configJSON(t, input))

	var server model.Server
	if err := json.Unmarshal(response.Body.Bytes(), &server); err != nil {
		t.Fatal(err)
	}

	endpoint := "/api/v1/servers/" + server.ID
	expectAccess(t, f.admin, "POST", endpoint+"/members", `{"username":"downloader"}`, http.StatusOK)
	f.runtime.files = func(input model.FileRequest) (model.FileResult, error) {
		if input.Offset > 0 {
			if err := f.auth.Access().RemoveMember(context.Background(), server.ID, user.ID); err != nil {
				t.Fatal(err)
			}
		}

		return model.FileResult{ServerID: input.ServerID, Data: []byte{1}, Size: 2}, nil
	}

	response = request(handler, "GET", endpoint+"/backups/game.tar.gz", "")
	if response.Header().Get("Content-Length") != "2" || response.Body.Len() != 1 {
		t.Fatal("download continued after access was revoked", response.Body.Len())
	}
}
