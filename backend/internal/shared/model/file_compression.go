package model

import "campfire/internal/shared/workprogress"

// Observable compression operations retain the synchronous compress contract.
const (
	FileOperationCompressStart  FileOperation = "compress.start"
	FileOperationCompressStatus FileOperation = "compress.status"
	FileOperationCompressCancel FileOperation = "compress.cancel"
)

// IsCompression identifies owner-bound compression session requests.
func (o FileOperation) IsCompression() bool {
	return o == FileOperationCompressStart || o == FileOperationCompressStatus || o == FileOperationCompressCancel
}

// IsFileControl allows observation/cancellation while lifecycle mutation is fenced.
func (o FileOperation) IsFileControl() bool {
	return o.IsExtractionStatusOrCancel() || o == FileOperationCompressStatus || o == FileOperationCompressCancel
}

// CompressionRequest identifies a short-lived idempotent archive operation.
type CompressionRequest struct {
	ID string `json:"id"`
}

// CompressionState describes the lifecycle of an owner-bound archive operation.
type CompressionState string

// Compression states distinguish a cancellation request from a drained writer.
const (
	CompressionRunning    CompressionState = "running"
	CompressionCancelling CompressionState = "cancelling"
	CompressionCompleted  CompressionState = "completed"
	CompressionCancelled  CompressionState = "cancelled"
	CompressionFailed     CompressionState = "failed"
)

// Terminal reports a confirmed outcome, not a pending cancellation request.
func (s CompressionState) Terminal() bool {
	return s == CompressionCompleted || s == CompressionCancelled || s == CompressionFailed
}

// CompressionStatus emits terminal states only after the writer has drained.
// Successful completion means the no-replace publication was acknowledged.
type CompressionStatus struct {
	ID           string                 `json:"id"`
	State        CompressionState       `json:"state"`
	Message      string                 `json:"message,omitempty"`
	WorkProgress *workprogress.Snapshot `json:"work_progress,omitempty"`
}
