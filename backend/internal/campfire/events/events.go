// Package events defines application facts independently of storage and delivery providers.
package events

import "campfire/internal/shared/model"

// Event is a typed application publication. Handlers accept these value types;
// an unsupported publication fails its transaction rather than disappearing.
type Event interface{ applicationEvent() }

// GameEventRecorded follows insertion of a retained game event.
type GameEventRecorded struct {
	ServerID string
	RecordID int64
	Event    model.GameEvent
}

// GameMediaRecorded follows a completed upload for an existing game event.
type GameMediaRecorded struct {
	GameEventRecorded
	Media model.GameMedia
}

// ConsoleAlertDetected contains both the retained detection and its server name.
// Notification handlers must exclude the detection's private console evidence.
type ConsoleAlertDetected struct {
	ServerID, ServerName string
	Alert                model.DetectedLogAlert
}

// OperationFinished follows the guarded transition to a terminal operation state.
// Commands and runtime error details are deliberately absent.
type OperationFinished struct {
	ServerID, OperationID string
	UserID                int64
	Username              string
	Action                model.ServerAction
	State                 model.ExecutionState
	BackupMode            model.BackupMode
}

// MonitoringAlertChanged reports a sustained monitoring condition transition.
// Notification formatting, severity and routing belong to publication handlers.
type MonitoringAlertChanged struct {
	ID                   string
	TargetType           model.NotificationTarget
	TargetID, Name, Code string
	Body                 string
	Active               bool
}

// JobStateChanged excludes checkpoints, payloads, digests and runtime errors.
type JobStateChanged struct {
	ID                             string
	Type                           model.JobType
	SourceServerID, TargetServerID string
	Previous, Current              model.JobStatus
	Attempt                        int64
}

// RequestStarted and RequestFinished contain allowlisted request facts for the
// audit handler. They never include request bodies or raw transport responses.
type RequestStarted struct {
	ID         string
	UserID     *int64
	Username   string
	APITokenID int64
	Action     model.AuditAction
	Target     string
	Outcome    model.AuditOutcome
	Status     int
}

// RequestFinished updates the durable request outcome after its handler returns.
type RequestFinished struct{ RequestStarted }

// PowerStateChanged follows a fresh observed transition or a confirmed restart.
type PowerStateChanged struct {
	ServerID          string
	Previous, Current model.RuntimeState
	Restarted         bool
}

// NotificationRequested preserves the existing inbox/Apprise notification policy
// for account, server, and Spark notifications. It does not imply integration delivery.
type NotificationRequested struct {
	ID         int64
	SourceKey  string
	TargetType model.NotificationTarget
	TargetID   string
	Kind       model.NotificationKind
	Severity   model.NotificationSeverity
	Title      string
	Body       string
}

func (GameEventRecorded) applicationEvent()      {}
func (GameMediaRecorded) applicationEvent()      {}
func (ConsoleAlertDetected) applicationEvent()   {}
func (OperationFinished) applicationEvent()      {}
func (PowerStateChanged) applicationEvent()      {}
func (NotificationRequested) applicationEvent()  {}
func (MonitoringAlertChanged) applicationEvent() {}
func (JobStateChanged) applicationEvent()        {}
func (RequestStarted) applicationEvent()         {}
func (RequestFinished) applicationEvent()        {}
