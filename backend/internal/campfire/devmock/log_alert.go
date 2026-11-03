//go:build !production

package devmock

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
	"time"

	"campfire/internal/shared/model"
)

func mockLogID(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func (record *runtimeRecord) beginLogExecution(phase model.LogPhase, operationID string) {
	if record.LogObservation == nil {
		record.LogObservation = &model.LogObservation{Executions: make(map[model.LogPhase]string), Alerts: []model.DetectedLogAlert{}}
	}

	record.LogObservation.Executions[phase] = mockLogID(record.ServerID + ":" + string(phase) + ":" + operationID)
	record.LogCursor = len(record.Logs)
}

// The simulator uses the same matcher and durable observation contract as Spark.
func (record *runtimeRecord) matchLogAlerts() bool {
	phase := model.LogRuntime
	if record.Operation == model.ServerActionInstall || record.Operation == model.ServerActionReinstall || record.Operation == model.ServerActionUpdate {
		phase = model.LogInstallation
	}

	return record.matchLogAlertsInPhase(phase)
}

func (record *runtimeRecord) configureLogAlerts(previous []model.LogAlert) {
	for _, rule := range record.Deployment.Ember.LogAlerts {
		if !slices.ContainsFunc(previous, func(old model.LogAlert) bool { return old.Key() == rule.Key() }) {
			record.LogCursor = 0
			break
		}
	}

	record.matchLogAlerts()
}

func (record *runtimeRecord) matchLogAlertsInPhase(phase model.LogPhase) bool {
	if len(record.Deployment.Ember.LogAlerts) == 0 {
		return false
	}

	initialized := record.LogObservation == nil
	if initialized {
		record.beginLogExecution(phase, "initial")
		record.LogCursor = 0
	}

	if record.LogCursor > len(record.Logs) {
		record.LogCursor = 0
	}

	if record.LogCursor == len(record.Logs) {
		return initialized
	}

	execution := record.LogObservation.Executions[phase]
	if execution == "" {
		execution = mockLogID(record.ServerID + ":" + string(phase) + ":initial")
		record.LogObservation.Executions[phase] = execution
	}

	for _, rule := range record.Deployment.Ember.LogAlerts {
		if rule.Phase != "" && rule.Phase != phase {
			continue
		}

		id := mockLogID(record.ServerID + ":" + execution + ":" + rule.Key())

		seen := false
		for _, existing := range record.LogObservation.Alerts {
			seen = seen || existing.ID == id
		}

		if seen {
			continue
		}

		matcher := model.CompileLogMatcher(rule.Any)

		for _, text := range strings.Split(record.Logs[record.LogCursor:], "\n") {
			stamp, line, ok := strings.Cut(text, " ")

			at, err := time.Parse(time.RFC3339Nano, stamp)
			if !ok || err != nil || !matcher.Matches(line) {
				continue
			}

			record.LogObservation.Alerts = append(record.LogObservation.Alerts, model.DetectedLogAlert{ID: id, RuleKey: rule.Key(), ExecutionID: execution, Phase: phase, Title: rule.Title, Message: rule.Message, Severity: rule.Severity, Action: rule.Action, DetectedAt: at.Unix(), Line: model.LogEvidence(line)})

			break
		}
	}

	record.LogObservation.Alerts = model.RetainLogAlerts(record.LogObservation.Alerts)

	record.LogCursor = len(record.Logs)

	return true
}
