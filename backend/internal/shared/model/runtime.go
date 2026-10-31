package model

// RuntimeState describes the server state exposed by Spark and the panel.
type RuntimeState string

// Runtime states are separate from Docker's container inspection states.
const (
	RuntimeStateNotInstalled RuntimeState = "not_installed"
	RuntimeStateInstalling   RuntimeState = "installing"
	RuntimeStateUpdating     RuntimeState = "updating"
	RuntimeStateReinstalling RuntimeState = "reinstalling"
	RuntimeStateStarting     RuntimeState = "starting"
	RuntimeStateRunning      RuntimeState = "running"
	RuntimeStateStopping     RuntimeState = "stopping"
	RuntimeStateStopped      RuntimeState = "stopped"
	RuntimeStateRestarting   RuntimeState = "restarting"
	RuntimeStateDeleting     RuntimeState = "deleting"
	RuntimeStateDeleted      RuntimeState = "deleted"
	RuntimeStateUnknown      RuntimeState = "unknown"
	RuntimeStateOffline      RuntimeState = "offline"
	RuntimeStateCreated      RuntimeState = "created"
	RuntimeStatePaused       RuntimeState = "paused"
	RuntimeStateExited       RuntimeState = "exited"
	RuntimeStateDead         RuntimeState = "dead"
	RuntimeStateRemoving     RuntimeState = "removing"
	RuntimeStateMissing      RuntimeState = "missing"
)

// ServerAction identifies a lifecycle operation or runtime inspection.
type ServerAction string

// Server actions keep their JSON values across panel and Spark transports.
const (
	ServerActionInstall   ServerAction = "install"
	ServerActionUpdate    ServerAction = "update"
	ServerActionReinstall ServerAction = "reinstall"
	ServerActionStart     ServerAction = "start"
	ServerActionStop      ServerAction = "stop"
	ServerActionRestart   ServerAction = "restart"
	ServerActionKill      ServerAction = "kill"
	ServerActionDelete    ServerAction = "delete"
	ServerActionStatus    ServerAction = "status"
	ServerActionLogs      ServerAction = "logs"
	ServerActionCommand   ServerAction = "command"
	ServerActionMetrics   ServerAction = "metrics"
	ServerActionBackup    ServerAction = "backup"
	ServerActionRestore   ServerAction = "restore"
	ServerActionTransfer  ServerAction = "transfer"
)

// OperationState identifies the lifecycle state of an asynchronous operation.
type OperationState string

// Operation lifecycle states are independent of their display messages.
const (
	OperationQueued      OperationState = "queued"
	OperationRunning     OperationState = "running"
	OperationSucceeded   OperationState = "succeeded"
	OperationFailed      OperationState = "failed"
	OperationInterrupted OperationState = "interrupted"
)

// RuntimeErrorCode identifies a runtime failure independently of its message.
type RuntimeErrorCode string

// Runtime failure codes are shared by Spark and the panel.
const (
	RuntimeOperationFailed      RuntimeErrorCode = "operation_failed"
	RuntimeOperationInterrupted RuntimeErrorCode = "operation_interrupted"
	RuntimeDockerFailed         RuntimeErrorCode = "docker_operation_failed"
	RuntimeInstallerFailed      RuntimeErrorCode = "installer_failed"
	RuntimeQuotaUnavailable     RuntimeErrorCode = "disk_quota_unavailable"
)

// RuntimeRequest identifies a server lifecycle operation; status is read-only.
type RuntimeRequest struct {
	ServerID     string        `json:"server_id"`
	Action       ServerAction  `json:"action"`
	OperationID  string        `json:"operation_id,omitempty"`
	Content      string        `json:"content,omitempty"`
	BackupSource *BackupSource `json:"backup_source,omitempty"`
}

// RuntimeStatus reports observed state and the latest asynchronous operation.
type RuntimeStatus struct {
	ServerID           string           `json:"server_id"`
	State              RuntimeState     `json:"state"`
	Ready              bool             `json:"ready"`
	OperationID        string           `json:"operation_id,omitempty"`
	Operation          ServerAction     `json:"operation,omitempty"`
	OperationState     OperationState   `json:"operation_state,omitempty"`
	Progress           string           `json:"progress,omitempty"`
	Error              string           `json:"error,omitempty"`
	ErrorCode          RuntimeErrorCode `json:"error_code,omitempty"`
	Busy               bool             `json:"busy"`
	DiskQuotaEnforced  bool             `json:"disk_quota_enforced"`
	DiskQuotaError     string           `json:"disk_quota_error,omitempty"`
	DiskQuotaErrorCode RuntimeErrorCode `json:"disk_quota_error_code,omitempty"`
	Logs               string           `json:"logs,omitempty"`
	Metrics            *ResourceUsage   `json:"metrics,omitempty"`
}
