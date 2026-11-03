//go:build !production

package devmock

import (
	"strings"
	"testing"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/devfixture"
	"campfire/internal/shared/model"
)

func TestLogAlertFixturePreservesPendingEmberChanges(t *testing.T) {
	_, db, _, _ := fixture(t)
	servers := repository.NewServerRepository(db.ORM())

	server, err := servers.Get(t.Context(), "creative")
	if err != nil {
		t.Fatal(err)
	}

	embers := repository.NewEmberRepository(db.ORM())

	ember, err := embers.Get(t.Context(), server.EmberID)
	if err != nil {
		t.Fatal(err)
	}

	var before runtimeRecord
	if err := db.ORM().Take(&before, "server_id = ?", server.ID).Error; err != nil {
		t.Fatal(err)
	}

	ember.StartupCommands["Standard"] = "echo pending startup change"
	if err := embers.Replace(t.Context(), ember); err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Delete(&seedMarker{ID: logAlertSeedMarkerID}).Error; err != nil {
		t.Fatal(err)
	}

	if err := seedLogAlertFixture(t.Context(), db.ORM()); err != nil {
		t.Fatal(err)
	}

	after, err := servers.Get(t.Context(), server.ID)
	if err != nil || !after.EmberUpdateAvailable || after.EmberHash != server.EmberHash {
		t.Fatal("fixture applied a pending Ember revision", after, err)
	}

	var record runtimeRecord
	if err := db.ORM().Take(&record, "server_id = ?", server.ID).Error; err != nil {
		t.Fatal(err)
	}

	if record.Deployment.Ember.Startup != before.Deployment.Ember.Startup || record.Operation != before.Operation || record.OperationID != before.OperationID {
		t.Fatal("fixture changed the pinned startup or operation", record.Deployment.Ember.Startup, record.Operation)
	}
}

func TestMockActionableErrorExamplePersistsDismissalAcrossRestart(t *testing.T) {
	runtime, db, auth, servers := fixture(t)

	var server model.Server

	for _, item := range servers {
		if item.ID == "creative" {
			server = item
		}
	}

	if server.ID != "creative" || server.EmberID != devfixture.CurrentMinecraftEmberID || server.EmberUpdateAvailable {
		t.Fatal("missing applied actionable-error example", server)
	}

	status, err := runtime.Action(t.Context(), server.SparkID, model.RuntimeRequest{ServerID: server.ID, Action: model.ServerActionStatus})
	if err != nil || status.State != model.RuntimeStateStopped || status.LogObservation == nil || len(status.LogObservation.Alerts) != 1 {
		t.Fatal("example has no stopped-server detection", status, err)
	}

	alert := status.LogObservation.Alerts[0]
	if alert.Action == nil || alert.Action.Type != model.LogOpenFile || alert.Action.Path != "eula.txt" || strings.Contains(alert.Line, "\x1b") {
		t.Fatal("example lost its file action or readable evidence", alert)
	}

	store := repository.NewLogAlertRepository(db.ORM())
	if err := store.Observe(t.Context(), server.SparkID, server.ID, *status.LogObservation, false); err != nil {
		t.Fatal(err)
	}

	if err := store.Dismiss(t.Context(), server.ID, alert.ID); err != nil {
		t.Fatal(err)
	}

	restored, err := New(t.Context(), db.ORM(), auth)
	if err != nil {
		t.Fatal(err)
	}

	status, err = restored.Action(t.Context(), server.SparkID, model.RuntimeRequest{ServerID: server.ID, Action: model.ServerActionStatus})
	if err != nil || status.LogObservation == nil || status.LogObservation.Alerts[0].ID != alert.ID {
		t.Fatal("restart changed the fixture occurrence", status, err)
	}

	if err := store.Observe(t.Context(), server.SparkID, server.ID, *status.LogObservation, false); err != nil {
		t.Fatal(err)
	}

	items, err := store.List(t.Context(), server.ID, true)
	if err != nil || len(items) != 0 {
		t.Fatal("restart undid dismissal", items, err)
	}
}
