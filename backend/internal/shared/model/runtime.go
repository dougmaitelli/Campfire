package model

// RuntimeRequest identifies a server lifecycle operation; status is read-only.
type RuntimeRequest struct {
	ServerID     string        `json:"server_id"`
	Action       string        `json:"action"`
	OperationID  string        `json:"operation_id,omitempty"`
	Content      string        `json:"content,omitempty"`
	BackupSource *BackupSource `json:"backup_source,omitempty"`
}

// RuntimeStatus reports observed state and the latest asynchronous operation.
type RuntimeStatus struct {
	ServerID          string         `json:"server_id"`
	State             string         `json:"state"`
	Ready             bool           `json:"ready"`
	OperationID       string         `json:"operation_id,omitempty"`
	Operation         string         `json:"operation,omitempty"`
	Progress          string         `json:"progress,omitempty"`
	Error             string         `json:"error,omitempty"`
	Busy              bool           `json:"busy"`
	DiskQuotaEnforced bool           `json:"disk_quota_enforced"`
	Logs              string         `json:"logs,omitempty"`
	Metrics           *ResourceUsage `json:"metrics,omitempty"`
}
