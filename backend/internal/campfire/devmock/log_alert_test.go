//go:build !production

package devmock

import (
	"testing"

	"campfire/internal/shared/model"
)

func TestRunningMockDetectsRulesAddedWithoutRestart(t *testing.T) {
	runtime, db, auth, _ := fixture(t)

	var record runtimeRecord
	if err := db.ORM().Take(&record, "server_id = ?", "survival").Error; err != nil {
		t.Fatal(err)
	}

	deployment := record.Deployment

	deployment.Ember.LogAlerts = []model.LogAlert{{Title: "Problem", Message: "Review settings", Severity: model.LogError, Any: []model.LogMatch{{Value: "new critical failure"}}}}
	if _, err := runtime.Deploy(t.Context(), record.SparkID, deployment); err != nil {
		t.Fatal(err)
	}

	status, err := runtime.Action(t.Context(), record.SparkID, model.RuntimeRequest{ServerID: record.ServerID, Action: model.ServerActionCommand, Content: "new critical failure"})
	if err != nil || status.LogObservation == nil || len(status.LogObservation.Alerts) != 1 {
		t.Fatal("first matching command was not detected", status, err)
	}

	firstID := status.LogObservation.Alerts[0].ID

	restored, err := New(t.Context(), db.ORM(), auth)
	if err != nil {
		t.Fatal(err)
	}

	status, err = restored.Action(t.Context(), record.SparkID, model.RuntimeRequest{ServerID: record.ServerID, Action: model.ServerActionCommand, Content: "new critical failure"})
	if err != nil || status.LogObservation == nil || len(status.LogObservation.Alerts) != 1 || status.LogObservation.Alerts[0].ID != firstID {
		t.Fatal("mock restart lost deduplication", status, err)
	}

	deployment.Ember.LogAlerts = append(deployment.Ember.LogAlerts, model.LogAlert{Title: "Additional rule", Message: "Review retained output", Severity: model.LogWarning, Any: []model.LogMatch{{Value: "new critical failure"}, {Value: "another failure"}}})
	if _, err := restored.Deploy(t.Context(), record.SparkID, deployment); err != nil {
		t.Fatal(err)
	}

	status, err = restored.Action(t.Context(), record.SparkID, model.RuntimeRequest{ServerID: record.ServerID, Action: model.ServerActionStatus})
	if err != nil || status.LogObservation == nil || len(status.LogObservation.Alerts) != 2 {
		t.Fatal("new condition did not replay retained output", status, err)
	}
}

func TestEmptyMockMonitoringInitializationIsPersistable(t *testing.T) {
	record := runtimeRecord{ServerID: "game", Deployment: model.Deployment{Ember: model.RuntimeDefinition{LogAlerts: []model.LogAlert{{Any: []model.LogMatch{{Value: "failure"}}}}}}}
	if !record.matchLogAlerts() || record.LogObservation == nil {
		t.Fatal("empty monitoring initialization was not marked as changed")
	}

	record.Logs = timestampLog("failure\n")
	if !record.matchLogAlerts() || len(record.LogObservation.Alerts) != 1 {
		t.Fatal("first output after initialization was skipped")
	}
}
