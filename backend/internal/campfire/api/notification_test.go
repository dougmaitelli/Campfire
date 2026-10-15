package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"campfire/internal/campfire/api"
	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
)

func TestNotificationsIsolationRevocationAndScopedTokens(t *testing.T) {
	db, err := database.Open(t.Context(), filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	auth := service.NewAuthService(repository.NewAuthRepository(db.ORM()))
	sparks := service.NewSparkService(repository.NewSparkRepository(db.ORM()), nil)
	embers := service.NewEmberService(repository.NewEmberRepository(db.ORM()))
	servers := service.NewServerService(repository.NewServerRepository(db.ORM()), embers)
	dispatch := &accessRuntime{}
	deployment := &service.DeploymentService{Servers: servers, Sparks: sparks, Embers: embers, Remote: dispatch}
	store := repository.NewNotificationRepository(db.ORM())
	notifications := service.NewNotificationService(store, deployment, auth)
	panel := api.New(sparks, embers, servers).WithDeployment(deployment).WithNotifications(notifications)
	admin := authenticatedTestAPI(t, db, panel)
	f := &accessFixture{t: t, auth: auth, admin: admin, panel: panel.Handler(), runtime: dispatch}
	user, member := f.user("recipient", database.UserRole)
	_, other := f.user("outsider", database.UserRole)
	node, _, input := serverFixture(t, admin)
	response := request(admin, "POST", "/api/v1/servers", configJSON(t, input))

	var server model.Server
	if err := json.Unmarshal(response.Body.Bytes(), &server); err != nil {
		t.Fatal(err)
	}

	endpoint := "/api/v1/servers/" + server.ID
	expectAccess(t, admin, "POST", endpoint+"/members", `{"username":"recipient","role_id":5}`, http.StatusOK)

	ctx := context.Background()

	item := database.Notification{TargetType: "server", TargetID: server.ID, Kind: "alert", Severity: "warning", Title: "Private server", Body: "Resource alert", CreatedAt: time.Now().Unix()}
	if err := store.Emit(ctx, &item, false); err != nil {
		t.Fatal(err)
	}

	sparkItem := database.Notification{TargetType: "spark", TargetID: node.ID, Kind: "alert", Severity: "failure", Title: "Private Spark", CreatedAt: time.Now().Unix()}
	if err := store.Emit(ctx, &sparkItem, false); err != nil {
		t.Fatal(err)
	}

	response = request(member, "GET", "/api/v1/notifications", "")
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "Private Spark") || !strings.Contains(response.Body.String(), "Private server") {
		t.Fatal("inbox visibility wrong", response.Code, response.Body)
	}

	if response := request(other, "GET", "/api/v1/notifications", ""); response.Code != http.StatusOK || response.Body.String() != "[]\n" {
		t.Fatal("another user's inbox leaked", response.Code, response.Body)
	}

	expectAccess(t, other, "PUT", fmt.Sprintf("/api/v1/notifications/%d/read", item.ID), `{"read":true}`, http.StatusNotFound)
	expectAccess(t, member, "GET", "/api/v1/notifications/status", "", http.StatusForbidden)
	expectAccess(t, member, "POST", "/api/v1/notifications/test", "", http.StatusForbidden)
	expectAccess(t, member, "PUT", fmt.Sprintf("/api/v1/notifications/%d/read", item.ID), `{"read":true}`, http.StatusNoContent)

	if response := request(member, "GET", "/api/v1/notifications/unread", ""); response.Body.String() != "{\"unread\":0}\n" {
		t.Fatal(response.Body)
	}

	if response := request(admin, "GET", "/api/v1/notifications/unread", ""); response.Body.String() != "{\"unread\":2}\n" {
		t.Fatal("recipient read state affected administrator", response.Body)
	}

	expectAccess(t, member, "PUT", fmt.Sprintf("/api/v1/notifications/%d/read", item.ID), `{"read":false}`, http.StatusNoContent)
	_, raw := createTestAPIToken(t, member, `["notifications.read","server.view"]`)
	token := bearerAPI(f.panel, raw)
	expectAccess(t, token, "GET", "/api/v1/notifications", "", http.StatusOK)
	expectAccess(t, token, "PUT", fmt.Sprintf("/api/v1/notifications/%d/read", item.ID), `{"read":true}`, http.StatusForbidden)

	_, raw = createTestAPIToken(t, member, `["notifications.read"]`)
	if response := request(bearerAPI(f.panel, raw), "GET", "/api/v1/notifications", ""); response.Body.String() != "[]\n" {
		t.Fatal("token scope bypassed server.view", response.Body)
	}

	expectAccess(t, admin, "DELETE", endpoint+"/members/"+fmt.Sprint(user.ID), "", http.StatusNoContent)

	if response := request(member, "GET", "/api/v1/notifications", ""); response.Body.String() != "[]\n" {
		t.Fatal("revoked notification access retained", response.Body)
	}

	expectAccess(t, member, "PUT", fmt.Sprintf("/api/v1/notifications/%d/read", item.ID), `{"read":true}`, http.StatusNotFound)
	expectAccess(t, admin, "POST", "/api/v1/notifications/test", "", http.StatusAccepted)

	if response := request(admin, "GET", "/api/v1/notifications/status", ""); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"apprise_enabled":false`) {
		t.Fatal(response.Code, response.Body)
	}
}
