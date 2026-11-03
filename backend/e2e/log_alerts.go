//go:build e2e && !production

package main

import "campfire/internal/shared/model"

// Simulated console commands produce output through the shared Ember matcher.
// Detection persistence, notifications and dismissal use the real panel services.
func (r *runtime) matchLogAlerts(input model.RuntimeRequest, status *model.RuntimeStatus) {
	rules := r.deployments[input.ServerID].Ember.LogAlerts
	if len(rules) == 0 {
		return
	}
	if status.LogObservation == nil {
		status.LogObservation = &model.LogObservation{Executions: make(map[model.LogPhase]string), Alerts: []model.DetectedLogAlert{}}
	}
	observation := status.LogObservation
	phase := model.LogRuntime
	switch input.Action {
	case model.ServerActionInstall, model.ServerActionReinstall, model.ServerActionUpdate:
		phase = model.LogInstallation
		fallthrough
	case model.ServerActionStart, model.ServerActionRestart:
		observation.Executions[phase] = revision(input.ServerID + ":" + string(phase) + ":" + input.OperationID)
	}
	if input.Action != model.ServerActionCommand {
		return
	}
	execution := observation.Executions[phase]
	for _, rule := range rules {
		if rule.Phase != "" && rule.Phase != phase {
			continue
		}
		id := revision(input.ServerID + ":" + execution + ":" + rule.Key())
		seen := false
		for _, previous := range observation.Alerts {
			seen = seen || previous.ID == id
		}
		if seen || !model.CompileLogMatcher(rule.Any).Matches(input.Content) {
			continue
		}
		observation.Alerts = append(observation.Alerts, model.DetectedLogAlert{ID: id, RuleKey: rule.Key(), ExecutionID: execution, Phase: phase, Title: rule.Title, Message: rule.Message, Severity: rule.Severity, Action: rule.Action, DetectedAt: fixed.Unix(), Line: model.LogEvidence(input.Content)})
	}
	observation.Alerts = model.RetainLogAlerts(observation.Alerts)
}
