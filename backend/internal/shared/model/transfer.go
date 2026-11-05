package model

// JobOperation identifies a fenced server-transfer operation on Spark.
type JobOperation string

// Transfer operations keep file transport separate from ordinary file management.
const (
	JobOperationStatus   JobOperation = "status"
	JobOperationExport   JobOperation = "export"
	JobOperationPrepare  JobOperation = "prepare"
	JobOperationRead     JobOperation = "read"
	JobOperationWrite    JobOperation = "write"
	JobOperationImport   JobOperation = "import"
	JobOperationActivate JobOperation = "activate"
	JobOperationRelease  JobOperation = "release"
	JobOperationDiscard  JobOperation = "discard"
	JobOperationCleanup  JobOperation = "cleanup"
)

// JobState describes durable progress on one Spark.
type JobState string

// Transfer states are separate from the panel's multi-node workflow phases.
const (
	JobStateAbsent    JobState = "absent"
	JobStateExporting JobState = "exporting"
	JobStateReady     JobState = "ready"
	JobStateReceiving JobState = "receiving"
	JobStateImporting JobState = "importing"
	JobStateImported  JobState = "imported"
	JobStatePublished JobState = "published"
	JobStateActive    JobState = "active"
	JobStateReleased  JobState = "released"
	JobStateRetired   JobState = "retired"
	JobStateDiscarded JobState = "discarded"
	JobStateCorrupt   JobState = "corrupt"
	JobStateFailed    JobState = "failed"
)

// TransferPhase identifies a panel-coordinated server-transfer workflow step.
type TransferPhase string

// Transfer phases remain stable while operations are retried or the panel restarts.
const (
	TransferPhaseQueued     TransferPhase = "queued"
	TransferPhaseExporting  TransferPhase = "exporting"
	TransferPhaseCopying    TransferPhase = "copying"
	TransferPhaseImporting  TransferPhase = "importing"
	TransferPhaseCommitting TransferPhase = "committing"
	TransferPhaseCleanup    TransferPhase = "cleanup"
	TransferPhaseCancelling TransferPhase = "cancelling"
	TransferPhaseCancelled  TransferPhase = "cancelled"
	TransferPhaseComplete   TransferPhase = "complete"
	TransferPhaseFailed     TransferPhase = "failed"
)

// JobRequest carries one bounded, idempotent transfer operation between
// Campfire and an authenticated Spark. Archives never pass through browser APIs.
// Attempt fences failed archive operations while keeping the archive identity stable.
type JobRequest struct {
	ArchiveFormat string       `json:"archive_format,omitempty"`
	ArchiveName   string       `json:"archive_name,omitempty"`
	ID            string       `json:"id"`
	Attempt       int64        `json:"attempt,omitempty"`
	ServerID      string       `json:"server_id"`
	Operation     JobOperation `json:"operation"`
	Offset        int64        `json:"offset,omitempty"`
	Size          int64        `json:"size,omitempty"`
	Digest        string       `json:"digest,omitempty"`
	Data          []byte       `json:"data,omitempty"`
}

// JobResult reports durable local progress or one immutable archive chunk.
type JobResult struct {
	ID       string   `json:"id"`
	ServerID string   `json:"server_id"`
	State    JobState `json:"state"`
	Error    string   `json:"error,omitempty"`
	Size     int64    `json:"size,omitempty"`
	Digest   string   `json:"digest,omitempty"`
	Offset   int64    `json:"offset,omitempty"`
	Data     []byte   `json:"data,omitempty"`
}
