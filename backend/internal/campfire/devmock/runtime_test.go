//go:build !production

package devmock

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
)

func fixture(t *testing.T) (*Runtime, *database.Database, *service.AuthService, []model.Server) {
	t.Helper()

	db, err := database.Open(t.Context(), filepath.Join(t.TempDir(), "mock.db"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	auth := service.NewAuthService(repository.NewAuthRepository(db.ORM()))

	runtime, err := New(t.Context(), db.ORM(), auth)
	if err != nil {
		t.Fatal(err)
	}

	servers, err := repository.NewServerRepository(db.ORM()).List(t.Context())
	if err != nil || len(servers) != 4 {
		t.Fatal("expected a seeded fleet", servers, err)
	}

	return runtime, db, auth, servers
}

func onlineServer(t *testing.T, runtime *Runtime, servers []model.Server) model.Server {
	t.Helper()

	for _, server := range servers {
		if !runtime.Connected(server.SparkID) {
			continue
		}

		status, err := runtime.Action(t.Context(), server.SparkID, model.RuntimeRequest{ServerID: server.ID, Action: "status"})
		if err != nil {
			t.Fatal(err)
		}

		if status.State == "running" {
			return server
		}
	}

	t.Fatal("missing running mock server")

	return model.Server{}
}

func TestSeedAuthenticationAndPersistence(t *testing.T) {
	runtime, db, auth, servers := fixture(t)

	_, token, err := auth.Login(t.Context(), Username, Password, "")
	if err != nil {
		t.Fatal("initial mock administrator cannot sign in", err)
	}

	session, _, err := auth.Session(t.Context(), token)
	if err != nil || session.User.RoleID != database.AdminRole || session.User.Name != "Developer" {
		t.Fatal("initial mock administrator cannot sign in", session, err)
	}

	server := onlineServer(t, runtime, servers)
	if _, err := runtime.Action(t.Context(), server.SparkID, model.RuntimeRequest{ServerID: server.ID, Action: "stop", OperationID: "test-stop"}); err != nil {
		t.Fatal(err)
	}

	reloaded, err := New(t.Context(), db.ORM(), auth)
	if err != nil {
		t.Fatal(err)
	}

	status, err := reloaded.Action(t.Context(), server.SparkID, model.RuntimeRequest{ServerID: server.ID, Action: "status"})
	if err != nil || status.State != "stopped" || status.OperationID != "test-stop" {
		t.Fatal("mock restart reset runtime state", status, err)
	}

	for _, record := range []struct {
		model any
		count int64
	}{
		{&database.User{}, 1}, {&database.Server{}, 4}, {&database.Spark{}, 3},
		{&database.Ember{}, 1}, {&database.Schedule{}, 1}, {&database.Notification{}, 2},
	} {
		var count int64
		if err := db.ORM().Model(record.model).Count(&count).Error; err != nil || count != record.count {
			t.Fatalf("mock restart duplicated fixtures for %T: %d, %v", record.model, count, err)
		}
	}
}

func TestLifecycleMetricsAndOffline(t *testing.T) {
	runtime, _, _, servers := fixture(t)
	server := onlineServer(t, runtime, servers)

	for _, action := range []string{"metrics", "stop", "metrics", "install", "start", "restart", "command", "logs"} {
		status, err := runtime.Action(t.Context(), server.SparkID, model.RuntimeRequest{ServerID: server.ID, Action: action, Content: "say hello", OperationID: "operation-" + action})
		if err != nil {
			t.Fatal(action, err)
		}

		if action == "metrics" && (status.Metrics == nil || status.Metrics.CPUPercent == nil || status.Metrics.MemoryBytes == nil || status.Metrics.SampledAt == 0) {
			t.Fatal("missing fresh resource readings", status)
		}

		if action == "logs" && !strings.Contains(status.Logs, "say hello") {
			t.Fatal("console input did not produce output", status.Logs)
		}

		if action == "restart" && (status.State != "running" || !status.Ready || status.OperationID != "operation-restart") {
			t.Fatal("runtime tracking contract was lost", status)
		}
	}

	for _, server := range servers {
		if runtime.Connected(server.SparkID) {
			continue
		}

		if _, err := runtime.Action(t.Context(), server.SparkID, model.RuntimeRequest{ServerID: server.ID, Action: "metrics"}); err == nil {
			t.Fatal("offline node returned fabricated live metrics")
		}
	}

	if _, err := runtime.Action(t.Context(), server.SparkID, model.RuntimeRequest{ServerID: server.ID, Action: "backup"}); err == nil {
		t.Fatal("unsupported backup pretended to succeed")
	}
}

func TestFileEditsAndRevisionsPersist(t *testing.T) {
	runtime, db, auth, servers := fixture(t)
	server := onlineServer(t, runtime, servers)
	request := model.FileRequest{ServerID: server.ID, Operation: "read", Path: "server.properties"}

	original, err := runtime.Files(t.Context(), server.SparkID, request)
	if err != nil {
		t.Fatal(err)
	}

	request.Operation, request.Revision, request.Data = "write", original.Revision, []byte("motd=Edited locally\n")
	if _, err := runtime.Files(t.Context(), server.SparkID, request); err != nil {
		t.Fatal(err)
	}

	if _, err := runtime.Files(t.Context(), server.SparkID, request); err == nil {
		t.Fatal("stale file editor overwrote a newer revision")
	}

	reloaded, err := New(t.Context(), db.ORM(), auth)
	if err != nil {
		t.Fatal(err)
	}

	request.Operation, request.Revision = "read", ""

	result, err := reloaded.Files(t.Context(), server.SparkID, request)
	if err != nil || string(result.Data) != string(request.Data) {
		t.Fatal("mock restart lost file edits", result, err)
	}

	request.Path = "../panel.db"
	if _, err := runtime.Files(t.Context(), server.SparkID, request); err == nil {
		t.Fatal("invalid path accepted")
	}
}

func TestConsoleDisconnectAndNewDeployments(t *testing.T) {
	runtime, _, _, servers := fixture(t)
	server := onlineServer(t, runtime, servers)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	output := ""

	err := runtime.Console(ctx, server.SparkID, server.ID, func(event model.ConsoleEvent) error {
		output += event.Content
		if event.Ready {
			cancel()
		}

		return nil
	})
	if !errors.Is(err, context.Canceled) || output == "" {
		t.Fatal("console did not stream and cancel cleanly", output, err)
	}

	deployment := model.Deployment{ServerID: "new-server", Configuration: server.ServerConfiguration}
	for range 2 {
		receipt, err := runtime.Deploy(t.Context(), server.SparkID, deployment)
		if err != nil || len(receipt.Revision) != 64 {
			t.Fatal("new server configuration failed", receipt, err)
		}
	}

	status, err := runtime.Action(t.Context(), server.SparkID, model.RuntimeRequest{ServerID: deployment.ServerID, Action: "status"})
	if err != nil || status.State != "not_installed" {
		t.Fatal("deploy implicitly installed a mock server", status, err)
	}
}
