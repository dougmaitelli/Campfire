package model

// CopyWork is an ephemeral projection of a protected copy job and its inputs.
type CopyWork struct {
	Digest              string              `json:"-"`
	ID                  string              `json:"id"`
	Attempt             int64               `json:"-"`
	SourceServerID      string              `json:"source_server_id"`
	TargetServerID      string              `json:"target_server_id"`
	SourceSparkID       string              `json:"source_spark_id"`
	TargetSparkID       string              `json:"target_spark_id"`
	SourceConfiguration ServerConfiguration `json:"-"`
	TargetConfiguration ServerConfiguration `json:"-"`
	Ember               Ember               `json:"-"`
	Phase               CopyPhase           `json:"phase"`
	Active              bool                `json:"active"`
	Error               string              `json:"error"`
	BackupID            string              `json:"-"`
	RestoreID           string              `json:"-"`
	DeleteID            string              `json:"-"`
	TransferredBytes    int64               `json:"transferred_bytes"`
	TotalBytes          int64               `json:"total_bytes"`
	CreatedAt           int64               `json:"created_at"`
	UpdatedAt           int64               `json:"updated_at"`
}

// ArchiveName identifies only this job's temporary portable full backup.
func (c CopyWork) ArchiveName() string { return CopyArchiveName(c.ID) }

// TransferWork is an ephemeral projection of a protected transfer job and its inputs.
type TransferWork struct {
	ID                  string              `json:"id"`
	Attempt             int64               `json:"-"`
	ServerID            string              `json:"server_id"`
	SourceSparkID       string              `json:"source_spark_id"`
	TargetSparkID       string              `json:"target_spark_id"`
	SourceConfiguration ServerConfiguration `json:"-"`
	TargetConfiguration ServerConfiguration `json:"-"`
	Phase               TransferPhase       `json:"phase"`
	ResumePhase         TransferPhase       `json:"-"`
	CancelFrom          TransferPhase       `json:"-"`
	Active              bool                `json:"active"`
	Error               string              `json:"error"`
	TransferredBytes    int64               `json:"transferred_bytes"`
	TotalBytes          int64               `json:"total_bytes"`
	Digest              string              `json:"-"`
	CreatedAt           int64               `json:"created_at"`
	UpdatedAt           int64               `json:"updated_at"`
}
