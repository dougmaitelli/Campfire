//go:build !production

package devmock

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

const (
	jobExamplesSeedMarkerID = 12
	pendingJobExampleID     = "mock-pending-transfer"
	runningJobExampleID     = "mock-running-copy"
	failedJobExampleID      = "mock-failed-transfer"
)

// EnableJobExamples adds development UI examples without creating any servers.
// Call before starting background jobs; existing examples retain user changes.
func (r *Runtime) EnableJobExamples(ctx context.Context) error {
	if err := seedJobExamples(ctx, r.db, r.now()); err != nil {
		return err
	}

	r.mu.Lock()
	r.jobExamples = true
	r.mu.Unlock()

	return nil
}

func seedJobExamples(ctx context.Context, db *gorm.DB, now time.Time) error {
	return database.Transaction(db.WithContext(ctx), func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&seedMarker{}).Where("id = ?", jobExamplesSeedMarkerID).Count(&count).Error; err != nil || count > 0 {
			return err
		}

		servers := repository.NewServerRepository(tx)

		examples := []struct {
			id, source, target string
			kind               model.JobType
			status             model.JobStatus
			step               model.JobStep
			age                time.Duration
			bytes, total       int64
			port               int
			error              string
		}{
			{id: pendingJobExampleID, source: "unavailable", target: "unavailable", kind: model.JobTransfer, status: model.JobPending, step: model.JobPreparing, age: 40 * time.Minute, port: 25574},
			{id: runningJobExampleID, source: "survival", target: "modded", kind: model.JobCopy, status: model.JobRunning, step: model.JobCopying, age: 6 * time.Minute, bytes: 640 << 20, total: 2 << 30},
			{id: failedJobExampleID, source: "creative", target: "creative", kind: model.JobTransfer, status: model.JobFailed, step: model.JobCopying, age: 2 * time.Hour, bytes: 256 << 20, total: 1 << 30, port: 25573, error: "Incoming archive checksum mismatch. Retry to copy the archive again."},
			{id: "mock-completed-copy", source: "survival", target: "modded", kind: model.JobCopy, status: model.JobCompleted, step: model.JobCleanup, age: 4 * time.Hour, bytes: 2 << 30, total: 2 << 30},
			{id: "mock-cancelled-copy", source: "survival", target: "unavailable", kind: model.JobCopy, status: model.JobCancelled, step: model.JobCopying, age: 8 * time.Hour, bytes: 128 << 20, total: 1 << 30},
			{id: "mock-local-copy", source: "survival", target: "creative", kind: model.JobCopy, status: model.JobCompleted, step: model.JobCleanup, age: 24 * time.Hour, bytes: 512 << 20, total: 512 << 20},
		}
		for _, example := range examples {
			source, err := servers.Get(ctx, example.source)
			if errors.Is(err, repository.ErrServerNotFound) {
				continue
			} else if err != nil {
				return err
			}

			target, err := servers.Get(ctx, example.target)
			if errors.Is(err, repository.ErrServerNotFound) {
				continue
			} else if err != nil {
				return err
			}

			job := database.ServerJob{
				ID: example.id, Type: example.kind, Status: example.status, Step: example.step, Attempt: 1, Data: "{}",
				SourceServerID: source.ID, TargetServerID: target.ID, SourceSparkID: source.SparkID, TargetSparkID: target.SparkID,
				CreatedAt: now.Add(-example.age).Unix(), UpdatedAt: now.Add(-time.Minute).Unix(),
				TransferredBytes: example.bytes, TotalBytes: example.total, Error: example.error,
			}
			if example.kind == model.JobTransfer {
				job.TargetSparkID = "mock-europe"
				if job.SourceSparkID == job.TargetSparkID {
					job.TargetSparkID = "mock-america"
				}

				ports := slices.Clone(source.Ports)
				if len(ports) > 0 {
					ports[0].HostPort = example.port
				}

				data, err := json.Marshal(model.TransferJobData{SourcePorts: source.Ports, TargetPorts: ports})
				if err != nil {
					return err
				}

				job.Data = string(data)
			}

			if !job.Active() {
				finished := job.CreatedAt + 3*60
				job.FinishedAt, job.UpdatedAt = &finished, finished
			}

			if err := tx.Create(&job).Error; err != nil {
				return err
			}
		}

		if err := tx.Model(&database.ServerJob{}).
			Where("id IN ? AND finished_at IS NULL", []string{"mock-completed-transfer", "mock-cancelled-transfer"}).
			Update("finished_at", gorm.Expr("updated_at")).Error; err != nil {
			return err
		}

		return tx.Create(&seedMarker{ID: jobExamplesSeedMarkerID}).Error
	})
}

type exampleJobHandler struct {
	next  service.JobHandler
	store *repository.JobRepository
}

func isJobExample(id string) bool {
	return id == pendingJobExampleID || id == runningJobExampleID || id == failedJobExampleID
}

// WrapJobHandler keeps example checkpoints available for inspection. User-created
// jobs use the normal workflow; cancelling an example never changes game files.
func (r *Runtime) WrapJobHandler(next service.JobHandler) service.JobHandler {
	r.mu.Lock()
	enabled := r.jobExamples
	r.mu.Unlock()

	if !enabled {
		return next
	}

	return exampleJobHandler{next: next, store: repository.NewJobRepository(r.db)}
}

func (h exampleJobHandler) Ready(job database.ServerJob) bool {
	if isJobExample(job.ID) {
		return job.Status == model.JobCancelling
	}

	return h.next.Ready(job)
}

func (h exampleJobHandler) Step(ctx context.Context, job database.ServerJob) (bool, error) {
	if isJobExample(job.ID) {
		return true, nil
	}

	return h.next.Step(ctx, job)
}

func (h exampleJobHandler) Cancel(ctx context.Context, job database.ServerJob) (bool, error) {
	if isJobExample(job.ID) {
		if job.Status != model.JobCancelling {
			return false, repository.ErrJobState
		}

		return false, h.store.Finish(ctx, job, true)
	}

	return h.next.Cancel(ctx, job)
}
