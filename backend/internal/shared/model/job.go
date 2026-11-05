package model

// JobType selects the workflow; transport and scheduling are shared.
type JobType string

// Supported background workflows.
const (
	JobCopy     JobType = "copy"
	JobTransfer JobType = "transfer"
)

// JobStatus describes scheduling and completion independently of the step.
type JobStatus string

// Job lifecycle statuses preserve reservations until completion or cancellation.
const (
	JobPending    JobStatus = "pending"
	JobRunning    JobStatus = "running"
	JobFailed     JobStatus = "failed"
	JobCancelling JobStatus = "cancelling"
	JobCompleted  JobStatus = "completed"
	JobCancelled  JobStatus = "cancelled"
)

// JobStep identifies the checkpoint preserved through failure and cancellation.
type JobStep string

// Job checkpoints are shared by the copy and transfer workflow handlers.
const (
	JobPreparing  JobStep = "preparing"
	JobExporting  JobStep = "exporting"
	JobCopying    JobStep = "copying"
	JobImporting  JobStep = "importing"
	JobCommitting JobStep = "committing"
	JobCleanup    JobStep = "cleanup"
)

// TransferJobData retains only placement information absent from the server row.
type TransferJobData struct {
	SourcePorts []ServerPort `json:"source_ports"`
	TargetPorts []ServerPort `json:"target_ports"`
}
