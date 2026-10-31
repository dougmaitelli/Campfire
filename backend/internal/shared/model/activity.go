package model

// ExecutionState describes scheduled or manually tracked panel operations.
type ExecutionState string

// Execution states track each attempt from dispatch through its terminal result.
const (
	ExecutionPending     ExecutionState = "pending"
	ExecutionQueued      ExecutionState = "queued"
	ExecutionDispatching ExecutionState = "dispatching"
	ExecutionRunning     ExecutionState = "running"
	ExecutionSucceeded   ExecutionState = "succeeded"
	ExecutionFailed      ExecutionState = "failed"
	ExecutionInterrupted ExecutionState = "interrupted"
	ExecutionCancelled   ExecutionState = "cancelled"
	ExecutionSkipped     ExecutionState = "skipped"
)

// AuditOutcome is separate from execution state because requests can be accepted.
type AuditOutcome string

// Audit outcomes include accepted requests whose operations finish separately.
const (
	AuditPending     AuditOutcome = "pending"
	AuditAccepted    AuditOutcome = "accepted"
	AuditSucceeded   AuditOutcome = "succeeded"
	AuditFailed      AuditOutcome = "failed"
	AuditInterrupted AuditOutcome = "interrupted"
	AuditCancelled   AuditOutcome = "cancelled"
	AuditSkipped     AuditOutcome = "skipped"
)

// AuditAction retains generic request names alongside known domain actions.
type AuditAction string

// ResourceAuditAction builds a namespaced action without changing stored values.
func ResourceAuditAction(resource, action string) AuditAction {
	return AuditAction(resource + "." + action)
}

// Scheduled reports whether the action is supported by recurring schedules.
func (a ServerAction) Scheduled() bool {
	switch a {
	case ServerActionStart, ServerActionStop, ServerActionRestart, ServerActionKill, ServerActionBackup, ServerActionCommand:
		return true
	default:
		return false
	}
}
