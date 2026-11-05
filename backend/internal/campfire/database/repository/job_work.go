package repository

import (
	"context"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"
)

type jobCheckpointKey struct{}

// WithJobCheckpoint carries the worker's observed generation through workflow
// adapters so their writes cannot acknowledge a different attempt.
func WithJobCheckpoint(ctx context.Context, job database.ServerJob) context.Context {
	return context.WithValue(ctx, jobCheckpointKey{}, job)
}

func expectedJob(ctx context.Context, store *JobRepository, id string) (database.ServerJob, error) {
	if job, ok := ctx.Value(jobCheckpointKey{}).(database.ServerJob); ok {
		if job.ID != id {
			return database.ServerJob{}, ErrJobState
		}

		return job, nil
	}

	return store.Get(ctx, id)
}

func copyStep(phase model.CopyPhase) model.JobStep {
	switch phase {
	case model.CopyPreparing:
		return model.JobPreparing
	case model.CopyBackup:
		return model.JobExporting
	case model.CopyUploading:
		return model.JobCopying
	case model.CopyRestoring:
		return model.JobImporting
	default:
		return model.JobCleanup
	}
}

func copyPhase(job database.ServerJob) model.CopyPhase {
	switch job.Status {
	case model.JobFailed:
		return model.CopyFailed
	case model.JobCancelling:
		return model.CopyCancelling
	case model.JobCancelled:
		return model.CopyCancelled
	case model.JobCompleted:
		return model.CopyComplete
	}

	return copyCheckpoint(job.Step)
}

func copyCheckpoint(step model.JobStep) model.CopyPhase {
	switch step {
	case model.JobPreparing:
		return model.CopyPreparing
	case model.JobExporting:
		return model.CopyBackup
	case model.JobCopying:
		return model.CopyUploading
	case model.JobImporting:
		return model.CopyRestoring
	default:
		return model.CopyCleanup
	}
}

func transferStep(phase model.TransferPhase) model.JobStep {
	switch phase {
	case model.TransferPhaseQueued:
		return model.JobPreparing
	case model.TransferPhaseExporting:
		return model.JobExporting
	case model.TransferPhaseCopying:
		return model.JobCopying
	case model.TransferPhaseImporting:
		return model.JobImporting
	case model.TransferPhaseCommitting:
		return model.JobCommitting
	default:
		return model.JobCleanup
	}
}

func transferCheckpoint(step model.JobStep) model.TransferPhase {
	switch step {
	case model.JobPreparing:
		return model.TransferPhaseQueued
	case model.JobExporting:
		return model.TransferPhaseExporting
	case model.JobCopying:
		return model.TransferPhaseCopying
	case model.JobImporting:
		return model.TransferPhaseImporting
	case model.JobCommitting:
		return model.TransferPhaseCommitting
	default:
		return model.TransferPhaseCleanup
	}
}

func transferPhase(job database.ServerJob) model.TransferPhase {
	switch job.Status {
	case model.JobFailed:
		return model.TransferPhaseFailed
	case model.JobCancelling:
		return model.TransferPhaseCancelling
	case model.JobCancelled:
		return model.TransferPhaseCancelled
	case model.JobCompleted:
		return model.TransferPhaseComplete
	}

	return transferCheckpoint(job.Step)
}

// copyWork projects protected records into execution inputs. Nothing in this
// projection is a persisted configuration or definition snapshot.
func (r *JobRepository) copyWork(ctx context.Context, job database.ServerJob) (model.CopyWork, error) {
	work := model.CopyWork{ID: job.ID, SourceServerID: job.SourceServerID, TargetServerID: job.TargetServerID, SourceSparkID: job.SourceSparkID, TargetSparkID: job.TargetSparkID, Phase: copyPhase(job), Active: job.Active(), Error: job.Error, BackupID: job.OperationID("export"), RestoreID: job.OperationID("import"), DeleteID: job.OperationID("discard"), TransferredBytes: job.TransferredBytes, TotalBytes: job.TotalBytes, CreatedAt: job.CreatedAt, UpdatedAt: job.UpdatedAt}

	work.Digest = job.Digest
	work.Attempt = job.Attempt

	if !job.Active() {
		return work, nil
	}

	source, err := NewServerRepository(r.db).Get(ctx, job.SourceServerID)
	if err != nil {
		return work, err
	}

	target, err := NewServerRepository(r.db).Get(ctx, job.TargetServerID)
	if err != nil {
		return work, err
	}

	ember, err := NewEmberRepository(r.db).Get(ctx, source.EmberID)
	if err != nil {
		return work, err
	}

	work.SourceConfiguration, work.TargetConfiguration, work.Ember = source.ServerConfiguration, target.ServerConfiguration, ember

	return work, nil
}

func (r *JobRepository) transferWork(ctx context.Context, job database.ServerJob) (model.TransferWork, error) {
	work := model.TransferWork{ID: job.ID, ServerID: job.SourceServerID, SourceSparkID: job.SourceSparkID, TargetSparkID: job.TargetSparkID, Phase: transferPhase(job), ResumePhase: transferCheckpoint(job.Step), CancelFrom: transferCheckpoint(job.Step), Active: job.Active(), Error: job.Error, TransferredBytes: job.TransferredBytes, TotalBytes: job.TotalBytes, Digest: job.Digest, CreatedAt: job.CreatedAt, UpdatedAt: job.UpdatedAt}

	work.Attempt = job.Attempt

	if !job.Active() {
		return work, nil
	}

	data, err := job.TransferData()
	if err != nil {
		return work, err
	}

	server, err := NewServerRepository(r.db).Get(ctx, job.SourceServerID)
	if err != nil {
		return work, err
	}

	work.SourceConfiguration, work.TargetConfiguration = server.ServerConfiguration, server.ServerConfiguration
	work.SourceConfiguration.SparkID, work.SourceConfiguration.Ports = job.SourceSparkID, data.SourcePorts
	work.TargetConfiguration.SparkID, work.TargetConfiguration.Ports = job.TargetSparkID, data.TargetPorts

	return work, nil
}
