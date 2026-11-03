package api_test

import (
	"encoding/json"
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

func TestLogAlertAPIPermissionsAndEvidence(t *testing.T) {
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
	store := repository.NewLogAlertRepository(db.ORM())
	deployment := &service.DeploymentService{Servers: servers, Sparks: sparks, Embers: embers, Remote: dispatch, LogAlerts: &service.LogAlertService{Store: store}}
	panel := api.New(sparks, embers, servers).WithDeployment(deployment)
	admin := authenticatedTestAPI(t, db, panel)
	f := &accessFixture{t: t, auth: auth, admin: admin, panel: panel.Handler(), runtime: dispatch}
	_, observer := f.user("log-observer", database.UserRole)
	_, outsider := f.user("log-outsider", database.UserRole)
	_, raw := createTestAPIToken(t, observer, `["server.view"]`)
	viewer := bearerAPI(f.panel, raw)
	node, _, input := serverFixture(t, admin)
	response := request(admin, "POST", "/api/v1/servers", configJSON(t, input))

	var server model.Server
	if err := json.Unmarshal(response.Body.Bytes(), &server); err != nil {
		t.Fatal(err)
	}

	endpoint := "/api/v1/servers/" + server.ID
	expectAccess(t, admin, "POST", endpoint+"/members", `{"username":"log-observer","role_id":5}`, http.StatusOK)

	alert := model.DetectedLogAlert{ID: strings.Repeat("a", 64), RuleKey: strings.Repeat("b", 64), ExecutionID: strings.Repeat("c", 64), Phase: model.LogRuntime, Title: "Action needed", Message: "Review settings", Severity: model.LogError, DetectedAt: time.Now().Unix(), Line: "secret-evidence"}
	if err := store.Observe(t.Context(), node.ID, server.ID, model.LogObservation{Executions: map[model.LogPhase]string{model.LogRuntime: alert.ExecutionID}, Alerts: []model.DetectedLogAlert{alert}}, false); err != nil {
		t.Fatal(err)
	}

	response = request(admin, "GET", endpoint+"/log-alerts", "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), alert.Line) {
		t.Fatal("admin lost issue evidence", response.Code, response.Body)
	}

	response = request(viewer, "GET", endpoint+"/log-alerts", "")
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), alert.Line) || !strings.Contains(response.Body.String(), alert.Title) {
		t.Fatal("view-only issue response exposed logs", response.Code, response.Body)
	}

	expectAccess(t, outsider, "GET", endpoint+"/log-alerts", "", http.StatusNotFound)
	expectAccess(t, viewer, "POST", endpoint+"/log-alerts/"+alert.ID+"/dismiss", `{}`, http.StatusForbidden)
	expectAccess(t, admin, "POST", endpoint+"/log-alerts/"+alert.ID+"/dismiss", `{}`, http.StatusNoContent)

	response = request(admin, "GET", endpoint+"/log-alerts", "")
	if response.Body.String() != "[]\n" {
		t.Fatal("dismissal did not remove active issue", response.Body)
	}
}
