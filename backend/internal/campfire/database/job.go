package database

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"campfire/internal/shared/model"
)

// ServerJob is the durable workflow checkpoint. Execution inputs live on servers
// and Embers, protected by the job until cleanup reaches a terminal status.
type ServerJob struct {
	ID               string          `gorm:"type:text;primaryKey;index:server_jobs_history,priority:2" json:"id"`
	Type             model.JobType   `gorm:"type:text;not null;check:server_jobs_type,type IN ('copy','transfer')" json:"type"`
	SourceServerID   string          `gorm:"type:text;not null;index" json:"source_server_id"`
	TargetServerID   string          `gorm:"type:text;not null;index" json:"target_server_id"`
	SourceSparkID    string          `gorm:"type:text;not null;index" json:"source_spark_id"`
	TargetSparkID    string          `gorm:"type:text;not null;index" json:"target_spark_id"`
	Data             string          `gorm:"type:text;not null;default:'{}'" json:"-"`
	Status           model.JobStatus `gorm:"type:text;not null;index;check:server_jobs_status,status IN ('pending','running','failed','cancelling','completed','cancelled')" json:"status"`
	Step             model.JobStep   `gorm:"type:text;not null;check:server_jobs_step,step IN ('preparing','exporting','copying','importing','committing','cleanup')" json:"step"`
	Attempt          int64           `gorm:"not null;default:1;check:server_jobs_attempt,attempt > 0" json:"-"`
	TransferredBytes int64           `gorm:"not null;default:0;check:server_jobs_progress,transferred_bytes >= 0 AND transferred_bytes <= total_bytes" json:"transferred_bytes"`
	TotalBytes       int64           `gorm:"not null;default:0;check:server_jobs_size,total_bytes >= 0" json:"total_bytes"`
	Digest           string          `gorm:"type:text;not null;default:''" json:"-"`
	Error            string          `gorm:"type:text;not null;default:''" json:"error"`
	CreatedAt        int64           `gorm:"not null;autoCreateTime:false;index:server_jobs_history,priority:1" json:"created_at"`
	UpdatedAt        int64           `gorm:"not null;autoUpdateTime:false" json:"updated_at"`
	FinishedAt       *int64          `gorm:"index" json:"finished_at"`
}

// Active reports whether the job still owns its reservations.
func (j ServerJob) Active() bool {
	return j.Status != model.JobCompleted && j.Status != model.JobCancelled
}

// OperationID is stable for acknowledgement replays and changes on a new attempt.
func (j ServerJob) OperationID(operation string) string {
	return fmt.Sprintf("%s-%s-%d", j.ID, operation, j.Attempt)
}

// TransferData decodes the transfer workflow's placement checkpoint.
func (j ServerJob) TransferData() (model.TransferJobData, error) {
	var data model.TransferJobData
	if j.Type != model.JobTransfer {
		return data, errors.New("job is not a transfer")
	}

	decoder := json.NewDecoder(bytes.NewBufferString(j.Data))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&data); err != nil {
		return data, fmt.Errorf("invalid transfer job data: %w", err)
	}

	if err := decoder.Decode(new(any)); err != io.EOF {
		return data, errors.New("transfer job data must contain one JSON object")
	}

	return data, nil
}
