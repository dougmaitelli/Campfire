package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"campfire/internal/campfire/api"
	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
)

type scheduleAPIRuntime struct {
	mu       sync.Mutex
	statuses map[string]model.RuntimeStatus
	calls    int
}

func (d *scheduleAPIRuntime) Deploy(_ context.Context, _ string, input model.Deployment) (model.ConfigurationReceipt, error) {
	return model.ConfigurationReceipt{ServerID: input.ServerID}, nil
}
func (d *scheduleAPIRuntime) Action(_ context.Context, _ string, input model.RuntimeRequest) (model.RuntimeStatus, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.statuses == nil {
		d.statuses = make(map[string]model.RuntimeStatus)
	}

	if input.Action == "status" {
		status := d.statuses[input.ServerID]
		status.ServerID = input.ServerID
		status.State = "stopped"

		return status, nil
	}

	d.calls++
	status := model.RuntimeStatus{ServerID: input.ServerID, State: "stopped", OperationID: input.OperationID, Operation: input.Action, Logs: input.Content}
	d.statuses[input.ServerID] = status

	return status, nil
}
func (d *scheduleAPIRuntime) Files(_ context.Context, _ string, input model.FileRequest) (model.FileResult, error) {
	return model.FileResult{ServerID: input.ServerID, Path: input.Path}, nil
}

func TestSchedulesExecutionAndAuditAPI(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	embers := service.NewEmberService(repository.NewEmberRepository(db.ORM()))
	servers := service.NewServerService(repository.NewServerRepository(db.ORM()), embers)
	sparks := service.NewSparkService(repository.NewSparkRepository(db.ORM()), nil)
	runtime := &scheduleAPIRuntime{}
	deployment := &service.DeploymentService{Servers: servers, Sparks: sparks, Embers: embers, Remote: runtime}
	auth := service.NewAuthService(repository.NewAuthRepository(db.ORM()))
	activity := service.NewActivityService(repository.NewActivityRepository(db.ORM()), deployment, auth)
	deployment.Activity = activity
	panel := api.New(sparks, embers, servers).WithDeployment(deployment).WithActivity(activity)

	admin := authenticatedTestAPI(t, db, panel)
	if err := activity.Start(ctx); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(activity.Close)
	_, _, input := serverFixture(t, admin)

	response := request(admin, "POST", "/api/v1/servers", configJSON(t, input))
	if response.Code != http.StatusCreated {
		t.Fatal(response.Code, response.Body)
	}

	var server model.Server
	if err := json.Unmarshal(response.Body.Bytes(), &server); err != nil {
		t.Fatal(err)
	}

	owner, err := auth.Access().CreateUser(ctx, "owner", testAdminPassword, database.UserRole, false, "", "")
	if err != nil {
		t.Fatal(err)
	}

	operator, err := auth.Access().CreateUser(ctx, "operator", testAdminPassword, database.UserRole, false, "", "")
	if err != nil {
		t.Fatal(err)
	}

	if err := auth.Access().TransferOwner(ctx, server.ID, owner.ID); err != nil {
		t.Fatal(err)
	}

	if err := auth.Access().Share(ctx, service.Principal{UserID: 1, Admin: true}, server.ID, operator.Username, database.OperatorRole); err != nil {
		t.Fatal(err)
	}

	userHandler := func(name string) http.Handler {
		session, token, err := auth.Login(ctx, name, testAdminPassword, "")
		if err != nil {
			t.Fatal(err)
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.AddCookie(&http.Cookie{Name: "campfire_session", Value: token})
			r.Header.Set("X-CSRF-Token", session.CSRF)
			panel.Handler().ServeHTTP(w, r)
		})
	}
	ownerHandler, operatorHandler := userHandler("owner"), userHandler("operator")

	const command = "private-scheduled-command"

	body := fmt.Sprintf(`{"server_id":%q,"name":"Nightly","action":"command","command":%q,"cron_expression":"0 3 * * *","time_zone":"UTC","enabled":true}`, server.ID, command)

	response = request(ownerHandler, "POST", "/api/v1/schedules", body)
	if response.Code != http.StatusCreated || strings.Contains(response.Body.String(), command) {
		t.Fatal("schedule create or redaction failed", response.Code, response.Body)
	}

	var schedule database.Schedule
	if err := json.Unmarshal(response.Body.Bytes(), &schedule); err != nil {
		t.Fatal(err)
	}

	expectAccess(t, operatorHandler, "POST", "/api/v1/schedules", body, http.StatusForbidden)
	expectAccess(t, operatorHandler, "PUT", "/api/v1/schedules/"+schedule.ID, body, http.StatusForbidden)
	expectAccess(t, operatorHandler, "PATCH", "/api/v1/schedules/"+schedule.ID, `{"enabled":false}`, http.StatusForbidden)
	expectAccess(t, ownerHandler, "PATCH", "/api/v1/schedules/"+schedule.ID, `{}`, http.StatusBadRequest)

	for _, enabled := range []bool{false, true} {
		response = request(ownerHandler, "PATCH", "/api/v1/schedules/"+schedule.ID, fmt.Sprintf(`{"enabled":%t}`, enabled))
		if response.Code != http.StatusOK || strings.Contains(response.Body.String(), command) {
			t.Fatal("toggle failed or exposed the command", response.Code, response.Body)
		}

		saved, err := activity.Store.Schedule(ctx, schedule.ID)
		if err != nil || saved.Enabled != enabled || saved.Command != command || saved.Name != schedule.Name || saved.CronExpression != schedule.CronExpression || saved.OnlyWhenRunning != schedule.OnlyWhenRunning || saved.TimeZone != schedule.TimeZone {
			t.Fatal("toggle modified unrelated schedule settings", saved, err)
		}
	}

	expectAccess(t, operatorHandler, "POST", "/api/v1/schedules/"+schedule.ID+"/run", "", http.StatusForbidden)
	expectAccess(t, operatorHandler, "GET", "/api/v1/audit", "", http.StatusForbidden)
	expectAccess(t, ownerHandler, "POST", "/api/v1/schedules/"+schedule.ID+"/run", "", http.StatusAccepted)

	deadline := time.Now().Add(5 * time.Second)

	for {
		response = request(ownerHandler, "GET", "/api/v1/schedules/executions", "")
		if response.Code != http.StatusOK {
			t.Fatal(response.Code, response.Body)
		}

		var runs []database.ScheduleRun
		if err := json.Unmarshal(response.Body.Bytes(), &runs); err != nil {
			t.Fatal(err)
		}

		if len(runs) == 1 && runs[0].State == "succeeded" {
			if runs[0].UserID != owner.ID || runs[0].Content != "" {
				t.Fatal("execution actor or redaction failed")
			}

			break
		}

		if time.Now().After(deadline) {
			t.Fatal("schedule did not execute", response.Body)
		}

		time.Sleep(20 * time.Millisecond)
	}

	response = request(ownerHandler, "POST", "/api/v1/servers/"+server.ID+"/actions", `{"action":"start"}`)
	if response.Code != http.StatusAccepted {
		t.Fatal("tracked manual action failed", response.Code, response.Body)
	}

	response = request(ownerHandler, "GET", "/api/v1/schedules/executions", "")

	var scheduleRuns []database.ScheduleRun

	if response.Code != http.StatusOK {
		t.Fatal(response.Code, response.Body)
	}

	if err := json.Unmarshal(response.Body.Bytes(), &scheduleRuns); err != nil {
		t.Fatal(err)
	}

	if len(scheduleRuns) != 1 || scheduleRuns[0].ScheduleID != schedule.ID {
		t.Fatal("schedule history included a direct operation", response.Body)
	}

	const password = "private-password-for-new-account"
	expectAccess(t, admin, "POST", "/api/v1/users", `{"username":"created","password":"`+password+`","role_id":3}`, http.StatusCreated)
	expectAccess(t, ownerHandler, "POST", "/api/v1/servers/"+server.ID+"/console", `{"command":"private-console-input"}`, http.StatusOK)
	expectAccess(t, ownerHandler, "POST", "/api/v1/servers/"+server.ID+"/files", `{"operation":"write","path":"secret.txt","data":"cHJpdmF0ZS1maWxl","revision":"absent"}`, http.StatusOK)
	token, tokenSecret := createTestAPIToken(t, ownerHandler, `["server.view","server.operate"]`)
	expectAccess(t, bearerAPI(panel.Handler(), tokenSecret), "POST", "/api/v1/servers/"+server.ID+"/console", `{"command":"private-token-command"}`, http.StatusOK)

	_, restricted := createTestAPIToken(t, admin, `["audit.view"]`)
	for _, path := range []string{"/api/v1/schedules", "/api/v1/schedules/executions"} {
		filtered := request(bearerAPI(panel.Handler(), restricted), "GET", path, "")
		if filtered.Code != http.StatusOK || filtered.Body.String() != "[]\n" {
			t.Fatal("scoped token leaked activity", filtered.Code, filtered.Body)
		}
	}

	response = request(panel.Handler(), "POST", "/api/v1/auth/login", `{"username":"owner","password":"private-invalid-password"}`)
	if response.Code != http.StatusUnauthorized {
		t.Fatal("invalid login unexpectedly succeeded", response.Code)
	}

	response = request(admin, "GET", "/api/v1/audit", "")
	if response.Code != http.StatusOK {
		t.Fatal(response.Code, response.Body)
	}

	for _, secret := range []string{command, password, tokenSecret, "private-token-command", "private-console-input", "cHJpdmF0ZS1maWxl", "private-invalid-password", "pbkdf2"} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatal("audit retained secret", secret)
		}
	}

	var events []database.AuditEvent
	if err := json.Unmarshal(response.Body.Bytes(), &events); err != nil {
		t.Fatal(err)
	}

	var completed, denied, login, tokenActor bool

	for _, event := range events {
		if event.Target == "server:"+server.ID && event.Outcome == "succeeded" && event.UserID != nil && *event.UserID == owner.ID && event.APITokenID == token.ID {
			tokenActor = true
		}

		if event.Action == "server.command" && event.Outcome == "succeeded" && event.UserID != nil && *event.UserID == owner.ID {
			completed = true
		}

		if event.Status == http.StatusForbidden && event.UserID != nil && *event.UserID == operator.ID {
			denied = true
		}

		if event.Action == "auth.login" && event.Outcome == "failed" {
			login = true
		}
	}

	if !completed || !denied || !login || !tokenActor {
		t.Fatal("audit missing completed, denied, login or token actor event", completed, denied, login, tokenActor)
	}

	expectAccess(t, ownerHandler, "PUT", "/api/v1/servers/"+server.ID, configJSON(t, input), http.StatusOK)

	response = request(admin, "GET", "/api/v1/audit?target=server:"+server.ID, "")
	if response.Code != http.StatusOK {
		t.Fatal(response.Code, response.Body)
	}

	if err := json.Unmarshal(response.Body.Bytes(), &events); err != nil {
		t.Fatal(err)
	}

	var configurationEdit bool

	for _, event := range events {
		if event.Target != "server:"+server.ID {
			t.Fatal("target filter leaked another resource")
		}

		if event.Action == "put.server" {
			configurationEdit = true
		}
	}

	if !configurationEdit {
		t.Fatal("server audit filter omitted configuration edits")
	}

	activity.Close()

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	response = request(ownerHandler, "POST", "/api/v1/servers/"+server.ID+"/actions", `{"action":"restart"}`)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatal("mutation accepted with audit storage unavailable", response.Code)
	}
}
