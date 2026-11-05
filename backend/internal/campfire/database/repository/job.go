package repository

import (
	"context"
	"errors"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

// ErrJobState rejects stale checkpoints and invalid job actions.
var ErrJobState = errors.New("job changed or does not permit this operation")

// ErrJobActive protects records reserved by an unfinished job.
var ErrJobActive = errors.New("finish or cancel the active server job first")

// JobRepository persists checkpoints and owns shared reservation guards.
type JobRepository struct{ db *gorm.DB }

// NewJobRepository constructs the durable workflow store.
func NewJobRepository(db *gorm.DB) *JobRepository { return &JobRepository{db: db} }

func unfinishedJobs(tx *gorm.DB) *gorm.DB {
	return tx.Model(&database.ServerJob{}).Where("status NOT IN ?", []model.JobStatus{model.JobCompleted, model.JobCancelled})
}

func guardJob(tx *gorm.DB, serverID string) error {
	var count int64
	if err := unfinishedJobs(tx).Where("source_server_id = ? OR target_server_id = ?", serverID, serverID).Count(&count).Error; err != nil {
		return err
	}

	if count != 0 {
		return ErrJobActive
	}

	return nil
}

// Guard rejects mutations involving an unfinished job.
func (r *JobRepository) Guard(ctx context.Context, serverID string) error {
	return guardJob(r.db.WithContext(ctx), serverID)
}

// GuardJobEmber protects definitions referenced by unfinished jobs.
func GuardJobEmber(tx *gorm.DB, emberID string) error {
	var count int64

	err := unfinishedJobs(tx).Where("source_server_id IN (?) OR target_server_id IN (?)", tx.Model(&database.Server{}).Select("id").Where("ember_id = ?", emberID), tx.Model(&database.Server{}).Select("id").Where("ember_id = ?", emberID)).Count(&count).Error
	if err != nil {
		return err
	}

	if count != 0 {
		return ErrJobActive
	}

	return nil
}

// Get loads one persisted job checkpoint.
func (r *JobRepository) Get(ctx context.Context, id string) (database.ServerJob, error) {
	var job database.ServerJob

	err := r.db.WithContext(ctx).Where("id = ?", id).Take(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = ErrAccessNotFound
	}

	return job, err
}

// Runnable excludes paused failures; cleanup and cancellation remain recoverable.
func (r *JobRepository) Runnable(ctx context.Context) ([]database.ServerJob, error) {
	jobs := make([]database.ServerJob, 0)
	// Repeated offline cleanup must not occupy every dispatch slot ahead of
	// healthy queued work. Erroring cancellations rotate by their last update.
	err := unfinishedJobs(r.db.WithContext(ctx)).Where("status <> ?", model.JobFailed).Order("CASE WHEN error = '' THEN 0 ELSE 1 END, updated_at, created_at, id").Find(&jobs).Error

	return jobs, err
}

// Advance compares the full checkpoint, including the confirmed byte offset.
// A late remote acknowledgement cannot overwrite cancellation or a newer attempt.
func (r *JobRepository) Advance(ctx context.Context, expected database.ServerJob, changes map[string]any) error {
	changes["updated_at"] = time.Now().Unix()

	result := r.db.WithContext(ctx).Model(&database.ServerJob{}).Where("id = ? AND status = ? AND step = ? AND attempt = ? AND transferred_bytes = ?", expected.ID, expected.Status, expected.Step, expected.Attempt, expected.TransferredBytes).Updates(changes)
	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected != 1 {
		return ErrJobState
	}

	return nil
}

// Change serializes user intent with worker checkpoints. Failed jobs continue
// holding their reservations and preserve the step which needs recovery.
func (r *JobRepository) Change(ctx context.Context, id string, cancel bool) error {
	return database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		store := NewJobRepository(tx)

		job, err := store.Get(ctx, id)
		if err != nil {
			return err
		}

		if !job.Active() {
			return ErrJobState
		}

		changes := map[string]any{"error": ""}

		if cancel {
			// Pending jobs have never dispatched runtime commands. The status CAS
			// fences a worker trying to start, so cancellation needs no connection.
			if job.Status == model.JobPending {
				if job.Type == model.JobCopy {
					return NewCopyRepository(tx).finishCancel(ctx, job)
				}

				return store.Finish(ctx, job, true)
			}

			if job.Status == model.JobCancelling || job.Step == model.JobCleanup || job.Type == model.JobTransfer && job.Step == model.JobCommitting {
				return ErrJobState
			}

			changes["status"] = model.JobCancelling
		} else {
			if job.Status != model.JobFailed {
				return ErrJobState
			}

			changes["status"], changes["attempt"] = model.JobRunning, job.Attempt+1
		}

		return store.Advance(ctx, job, changes)
	})
}

// JobCursor identifies an exclusive position in chronological history.
type JobCursor struct {
	CreatedAt int64  `json:"created_at"`
	ID        string `json:"id"`
}

// JobHistory adds display names without loading configurations or definitions.
// Missing endpoints retain their IDs so deleted resources do not erase history.
type JobHistory struct {
	database.ServerJob
	SourceName      string `json:"source_name"`
	TargetName      string `json:"target_name"`
	SourceSparkName string `json:"source_spark_name"`
	TargetSparkName string `json:"target_spark_name"`
}

func (r *JobRepository) history(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Model(&database.ServerJob{}).
		Select("server_jobs.*, COALESCE(source.name, server_jobs.source_server_id) AS source_name, COALESCE(target.name, server_jobs.target_server_id) AS target_name, COALESCE(source_spark.name, server_jobs.source_spark_id) AS source_spark_name, COALESCE(target_spark.name, server_jobs.target_spark_id) AS target_spark_name").
		Joins("LEFT JOIN servers source ON source.id = server_jobs.source_server_id").
		Joins("LEFT JOIN servers target ON target.id = server_jobs.target_server_id").
		Joins("LEFT JOIN sparks source_spark ON source_spark.id = server_jobs.source_spark_id").
		Joins("LEFT JOIN sparks target_spark ON target_spark.id = server_jobs.target_spark_id")
}

// HistoryItem loads the same display projection used in paginated history.
func (r *JobRepository) HistoryItem(ctx context.Context, id string) (JobHistory, error) {
	var job JobHistory

	err := r.history(ctx).Where("server_jobs.id = ?", id).Take(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = ErrAccessNotFound
	}

	return job, err
}

// ActiveForServer reads only unfinished checkpoints, including paused failures.
func (r *JobRepository) ActiveForServer(ctx context.Context, serverID string, jobType model.JobType) ([]database.ServerJob, error) {
	jobs := make([]database.ServerJob, 0)
	err := unfinishedJobs(r.db.WithContext(ctx)).Where("type = ? AND (source_server_id = ? OR target_server_id = ?)", jobType, serverID, serverID).Find(&jobs).Error

	return jobs, err
}

// List reads a stable descending page. Permissions are applied before paging by
// the caller-provided set of visible workflow types.
func (r *JobRepository) List(ctx context.Context, serverID string, types []model.JobType, before *JobCursor) ([]JobHistory, error) {
	if len(types) == 0 {
		return []JobHistory{}, nil
	}

	query := r.history(ctx).Where("(source_server_id = ? OR target_server_id = ?) AND type IN ?", serverID, serverID, types)

	return r.historyPage(query, before)
}

// JobFilter selects workflow and lifecycle state across all servers.
type JobFilter struct {
	Type   model.JobType
	Status model.JobStatus
}

// ListAll is the platform history projection. Authorization belongs to the API.
func (r *JobRepository) ListAll(ctx context.Context, filter JobFilter, before *JobCursor) ([]JobHistory, error) {
	query := r.history(ctx)
	if filter.Type != "" {
		query = query.Where("server_jobs.type = ?", filter.Type)
	}

	if filter.Status != "" {
		query = query.Where("server_jobs.status = ?", filter.Status)
	}

	return r.historyPage(query, before)
}

func (r *JobRepository) historyPage(query *gorm.DB, before *JobCursor) ([]JobHistory, error) {
	jobs := make([]JobHistory, 0)

	if before != nil {
		query = query.Where("server_jobs.created_at < ? OR (server_jobs.created_at = ? AND server_jobs.id < ?)", before.CreatedAt, before.CreatedAt, before.ID)
	}

	err := query.Order("server_jobs.created_at DESC, server_jobs.id DESC").Limit(51).Find(&jobs).Error

	return jobs, err
}

// Finish records a terminal result without deleting its history.
func (r *JobRepository) Finish(ctx context.Context, job database.ServerJob, cancelled bool) error {
	status := model.JobCompleted
	if cancelled {
		status = model.JobCancelled
	}

	return r.Advance(ctx, job, map[string]any{"status": status, "error": "", "finished_at": time.Now().Unix()})
}
