package model

// TransferOperation identifies a fenced server-transfer operation on Spark.
type TransferOperation string

// Transfer operations keep file transport separate from ordinary file management.
const (
	TransferOperationStatus   TransferOperation = "status"
	TransferOperationExport   TransferOperation = "export"
	TransferOperationPrepare  TransferOperation = "prepare"
	TransferOperationRead     TransferOperation = "read"
	TransferOperationWrite    TransferOperation = "write"
	TransferOperationImport   TransferOperation = "import"
	TransferOperationActivate TransferOperation = "activate"
	TransferOperationRelease  TransferOperation = "release"
	TransferOperationDiscard  TransferOperation = "discard"
	TransferOperationCleanup  TransferOperation = "cleanup"
)

// TransferState describes durable progress on one Spark.
type TransferState string

// Transfer states are separate from the panel's multi-node workflow phases.
const (
	TransferStateAbsent    TransferState = "absent"
	TransferStateExporting TransferState = "exporting"
	TransferStateReady     TransferState = "ready"
	TransferStateReceiving TransferState = "receiving"
	TransferStateImporting TransferState = "importing"
	TransferStateImported  TransferState = "imported"
	TransferStateActive    TransferState = "active"
	TransferStateReleased  TransferState = "released"
	TransferStateRetired   TransferState = "retired"
	TransferStateDiscarded TransferState = "discarded"
	TransferStateCorrupt   TransferState = "corrupt"
	TransferStateFailed    TransferState = "failed"
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

// TransferRequest carries one bounded, idempotent transfer operation between
// Campfire and an authenticated Spark. Archives never pass through browser APIs.
type TransferRequest struct {
	ID        string            `json:"id"`
	ServerID  string            `json:"server_id"`
	Operation TransferOperation `json:"operation"`
	Offset    int64             `json:"offset,omitempty"`
	Size      int64             `json:"size,omitempty"`
	Digest    string            `json:"digest,omitempty"`
	Data      []byte            `json:"data,omitempty"`
}

// TransferResult reports durable local progress or one immutable archive chunk.
type TransferResult struct {
	ID       string        `json:"id"`
	ServerID string        `json:"server_id"`
	State    TransferState `json:"state"`
	Error    string        `json:"error,omitempty"`
	Size     int64         `json:"size,omitempty"`
	Digest   string        `json:"digest,omitempty"`
	Offset   int64         `json:"offset,omitempty"`
	Data     []byte        `json:"data,omitempty"`
}
