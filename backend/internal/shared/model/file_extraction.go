package model

import "time"

// ExtractionAction identifies an explicit answer to a destination conflict.
type ExtractionAction string

// Extraction actions retain their JSON values across panel and Spark transports.
const (
	ExtractionActionSkip      ExtractionAction = "skip"
	ExtractionActionRename    ExtractionAction = "rename"
	ExtractionActionOverwrite ExtractionAction = "overwrite"
	ExtractionActionMerge     ExtractionAction = "merge"
	ExtractionActionExtract   ExtractionAction = "extract"
)

// ExtractionState identifies the operation lifecycle, separately from entries.
type ExtractionState string

// Extraction states distinguish active work, human waits and terminal results.
const (
	ExtractionRunning   ExtractionState = "running"
	ExtractionWaiting   ExtractionState = "waiting"
	ExtractionCompleted ExtractionState = "completed"
	ExtractionCancelled ExtractionState = "cancelled"
	ExtractionExpired   ExtractionState = "expired"
	ExtractionFailed    ExtractionState = "failed"
)

// Terminal reports whether no further conflict answers can be accepted.
func (s ExtractionState) Terminal() bool {
	return s == ExtractionCompleted || s == ExtractionCancelled || s == ExtractionExpired || s == ExtractionFailed
}

// ExtractionOutcomeStatus contains only statuses sent by Spark. Browser-only
// uploaded/unconfirmed publication results are not extraction protocol values.
type ExtractionOutcomeStatus string

// Extraction outcomes report encountered entries, including undecided members.
const (
	ExtractionOutcomePending     ExtractionOutcomeStatus = "pending"
	ExtractionOutcomeExtracted   ExtractionOutcomeStatus = "extracted"
	ExtractionOutcomeMerged      ExtractionOutcomeStatus = "merged"
	ExtractionOutcomeOverwritten ExtractionOutcomeStatus = "overwritten"
	ExtractionOutcomeRenamed     ExtractionOutcomeStatus = "renamed"
	ExtractionOutcomeSkipped     ExtractionOutcomeStatus = "skipped"
	ExtractionOutcomeFailed      ExtractionOutcomeStatus = "failed"
	ExtractionOutcomeCancelled   ExtractionOutcomeStatus = "cancelled"
)

// ExtractionEntry describes an archive member encountered during extraction.
type ExtractionEntry struct {
	Path string   `json:"path"`
	Kind FileKind `json:"kind"`
	Size int64    `json:"size"`
}

// ExtractionConflict identifies one destination observation, not an upfront plan.
type ExtractionConflict struct {
	ID          string     `json:"id"`
	Path        string     `json:"path"`
	Destination string     `json:"destination"`
	Kind        FileKind   `json:"kind"`
	Size        int64      `json:"size"`
	Existing    *FileEntry `json:"existing,omitempty"`
	Changed     bool       `json:"changed,omitempty"`
	Protected   bool       `json:"protected,omitempty"`
}

// ExtractionRequest advances or resolves an owner-bound extraction operation.
type ExtractionRequest struct {
	ID         string           `json:"id"`
	ConflictID string           `json:"conflict_id,omitempty"`
	Action     ExtractionAction `json:"action,omitempty"`
	Name       string           `json:"name,omitempty"`
}

// ExtractionStatus is a bounded snapshot. Polling never renews its deadline.
type ExtractionStatus struct {
	ID               string              `json:"id"`
	Version          int64               `json:"version"`
	State            ExtractionState     `json:"state"`
	ExpiresAt        time.Time           `json:"expires_at"`
	Conflict         *ExtractionConflict `json:"conflict,omitempty"`
	PendingConflicts int                 `json:"pending_conflicts"`
	RemainingEntries int                 `json:"remaining_entries"`
	Outcomes         []FileOutcome       `json:"outcomes"`
	Message          string              `json:"message,omitempty"`
}

// FileOutcome reports each written, skipped, failed or still-pending entry.
type FileOutcome struct {
	Path        string                  `json:"path"`
	Destination string                  `json:"destination"`
	Status      ExtractionOutcomeStatus `json:"status"`
	Message     string                  `json:"message,omitempty"`
}

// IsExtraction identifies the conflict-driven extraction protocol.
func (o FileOperation) IsExtraction() bool {
	return o == FileOperationExtractStart || o == FileOperationExtractStatus || o == FileOperationExtractResolve || o == FileOperationExtractCancel
}

// IsExtractionStatusOrCancel identifies control requests that must remain
// available for an existing operation even while a transfer fences new writes.
func (o FileOperation) IsExtractionStatusOrCancel() bool {
	return o == FileOperationExtractStatus || o == FileOperationExtractCancel
}

// IsArchive identifies potentially long-running archive work.
func (o FileOperation) IsArchive() bool {
	return o == FileOperationCompress || o == FileOperationExtract || o == FileOperationExtractStart
}
