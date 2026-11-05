package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

func (s *CopyService) step(ctx context.Context, job model.CopyWork) (bool, error) {
	next := func(phase model.CopyPhase) (bool, error) {
		return false, s.Store.Advance(ctx, job.ID, job.Phase, repository.CopyCheckpoint{Phase: phase})
	}

	switch job.Phase {
	case model.CopyPreparing:
		deployments := make([]model.Deployment, 0, 2)

		for i, configuration := range []model.ServerConfiguration{job.SourceConfiguration, job.TargetConfiguration} {
			id := job.SourceServerID
			if i == 1 {
				id = job.TargetServerID
			}

			deployment, err := model.NewDeployment(id, configuration, job.Ember)
			if err != nil {
				return false, err
			}

			if err := s.Deployment.checkJobCapabilities(deployment, model.JobCopy); err != nil {
				return false, err
			}

			deployments = append(deployments, deployment)
		}

		for _, deployment := range deployments {
			if _, err := s.Deployment.Remote.Deploy(ctx, deployment.Configuration.SparkID, deployment); err != nil {
				return false, err
			}
		}

		return next(model.CopyBackup)
	case model.CopyBackup:
		status, err := s.action(ctx, job.SourceSparkID, model.RuntimeRequest{ServerID: job.SourceServerID, Action: model.ServerActionStatus})
		if err != nil {
			return false, err
		}

		if status.Busy {
			return true, nil
		}

		archive, err := s.archive(ctx, job)
		if err != nil {
			return false, err
		}

		if archive != nil {
			digest, err := s.archiveDigest(ctx, job, archive.Size)
			if err != nil {
				return false, err
			}

			return false, s.Store.Advance(ctx, job.ID, job.Phase, repository.CopyCheckpoint{Phase: model.CopyUploading, Archive: &repository.CopyArchive{Size: archive.Size, Digest: digest}})
		}

		if status.OperationID == job.BackupID {
			if status.OperationState == model.OperationInterrupted {
				return false, s.Store.Advance(ctx, job.ID, job.Phase, repository.CopyCheckpoint{RestartOperation: true})
			}

			if status.Error != "" {
				return false, errors.New(status.Error)
			}

			return false, errors.New("backup operation finished without a portable archive")
		}

		_, err = s.action(ctx, job.SourceSparkID, model.RuntimeRequest{ServerID: job.SourceServerID, Action: model.ServerActionBackup, OperationID: job.BackupID, ArchiveID: job.ID, BackupOptions: model.BackupOptions{Mode: model.BackupModeFull, EmberRevision: model.EmberHash(job.Ember), Definition: model.NewBackupDefinition(model.Server{ServerConfiguration: job.SourceConfiguration}, job.Ember)}})

		return true, err
	case model.CopyUploading:
		return s.upload(ctx, job)
	case model.CopyRestoring:
		status, err := s.action(ctx, job.TargetSparkID, model.RuntimeRequest{ServerID: job.TargetServerID, Action: model.ServerActionStatus})
		if err != nil {
			return false, err
		}

		if status.Busy {
			return true, nil
		}

		completedEarlierAttempt := strings.HasPrefix(status.OperationID, job.ID+"-import-") && status.OperationState == model.OperationSucceeded
		if status.OperationID == job.RestoreID || completedEarlierAttempt {
			if status.OperationState == model.OperationInterrupted {
				return false, s.Store.Advance(ctx, job.ID, job.Phase, repository.CopyCheckpoint{RestartOperation: true})
			}

			if status.Error != "" {
				return false, errors.New(status.Error)
			}

			if status.OperationState != model.OperationSucceeded || status.State != model.RuntimeStateStopped {
				return false, errors.New("copy restore did not finish successfully")
			}

			return next(model.CopyCleanup)
		}

		_, err = s.action(ctx, job.TargetSparkID, model.RuntimeRequest{ServerID: job.TargetServerID, Action: model.ServerActionRestore, OperationID: job.RestoreID, Content: job.ArchiveName()})

		return true, err
	case model.CopyCleanup, model.CopyCancelling:
		return s.cleanup(ctx, job)
	default:
		return false, errors.New("invalid persisted copy phase")
	}
}

func (s *CopyService) archiveDigest(ctx context.Context, job model.CopyWork, size int64) (string, error) {
	hash := sha256.New()

	for offset := int64(0); offset < size; {
		chunk, err := s.files(ctx, job.SourceSparkID, job.SourceServerID, model.FileRequest{Operation: model.FileOperationBackupRead, Path: job.ArchiveName(), Offset: offset})
		if err != nil {
			return "", err
		}

		if chunk.Size != size || len(chunk.Data) == 0 || int64(len(chunk.Data)) > size-offset {
			return "", errors.New("source archive changed while hashing")
		}

		_, _ = hash.Write(chunk.Data)
		offset += int64(len(chunk.Data))
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (s *CopyService) upload(ctx context.Context, job model.CopyWork) (bool, error) {
	remote, ok := s.Deployment.Remote.(JobDispatcher)
	if !ok {
		return false, ErrDeploymentUnavailable
	}

	request := model.JobRequest{ID: job.ID, Attempt: job.Attempt, ServerID: job.TargetServerID, ArchiveFormat: "backup", ArchiveName: job.ArchiveName(), Size: job.TotalBytes, Digest: job.Digest, Operation: model.JobOperationPrepare}

	observed, err := remote.Job(ctx, job.TargetSparkID, request)
	if err != nil {
		return false, err
	}

	if observed.State == model.JobStatePublished {
		return false, s.Store.Advance(ctx, job.ID, job.Phase, repository.CopyCheckpoint{Phase: model.CopyRestoring, Offset: &job.TotalBytes})
	}

	if observed.Offset > job.TotalBytes || observed.Offset < 0 {
		return false, errors.New("invalid durable archive offset")
	}

	// Import is idempotent within an attempt. A new attempt resumes failed
	// publication using the retained archive, including after a Spark restart.
	if observed.State == model.JobStateImporting || observed.State == model.JobStateFailed || observed.Offset == job.TotalBytes {
		request.Operation = model.JobOperationImport

		result, err := remote.Job(ctx, job.TargetSparkID, request)
		if err != nil {
			return false, err
		}

		if result.State == model.JobStateFailed {
			return false, errors.New(result.Error)
		}

		return true, nil
	}

	chunk, err := s.files(ctx, job.SourceSparkID, job.SourceServerID, model.FileRequest{Operation: model.FileOperationBackupRead, Path: job.ArchiveName(), Offset: observed.Offset})
	if err != nil {
		return false, err
	}

	if chunk.Size != job.TotalBytes || len(chunk.Data) == 0 || len(chunk.Data) > model.FileLimit || int64(len(chunk.Data)) > job.TotalBytes-observed.Offset {
		return false, errors.New("source copy archive changed")
	}

	request.Operation, request.Offset, request.Data = model.JobOperationWrite, observed.Offset, chunk.Data

	ack, err := remote.Job(ctx, job.TargetSparkID, request)
	if err != nil {
		return false, err
	}

	if ack.Offset != observed.Offset+int64(len(chunk.Data)) {
		return false, errors.New("destination copy offset mismatch")
	}

	return false, s.Store.Advance(ctx, job.ID, job.Phase, repository.CopyCheckpoint{Offset: &ack.Offset})
}

func (s *CopyService) cleanup(ctx context.Context, job model.CopyWork) (bool, error) {
	targetAbsent := false

	if job.Phase == model.CopyCleanup {
		remote, ok := s.Deployment.Remote.(JobDispatcher)
		if !ok {
			return false, ErrDeploymentUnavailable
		}

		if _, err := remote.Job(ctx, job.TargetSparkID, model.JobRequest{ID: job.ID, ServerID: job.TargetServerID, Operation: model.JobOperationActivate}); err != nil {
			return false, err
		}
	}

	if job.Phase == model.CopyCancelling {
		remote, ok := s.Deployment.Remote.(JobDispatcher)
		if !ok {
			return false, ErrDeploymentUnavailable
		}

		// Discard also handles a destination which never received configuration.
		// Cleanup must not depend on deploying a possibly unsupported definition.
		query := model.JobRequest{ID: job.ID, ServerID: job.TargetServerID, ArchiveFormat: "backup", Operation: model.JobOperationDiscard}

		observed, err := remote.Job(ctx, job.TargetSparkID, query)
		if err != nil {
			return false, err
		}

		if observed.State != model.JobStateAbsent && observed.State != model.JobStateDiscarded {
			return true, nil
		}

		targetAbsent = observed.State == model.JobStateAbsent
	}

	type endpoint struct{ spark, server string }

	endpoints := []endpoint{{job.SourceSparkID, job.SourceServerID}}
	if !targetAbsent {
		endpoints = append(endpoints, endpoint{job.TargetSparkID, job.TargetServerID})
	}
	// Existing backup/restore operations are asynchronous. Wait for them rather
	// than removing storage out from underneath a still-running operation.
	for _, endpoint := range endpoints {
		status, err := s.action(ctx, endpoint.spark, model.RuntimeRequest{ServerID: endpoint.server, Action: model.ServerActionStatus})
		if err != nil {
			return false, err
		}

		if status.Busy {
			return true, nil
		}
	}

	for _, endpoint := range endpoints {
		_, err := s.files(ctx, endpoint.spark, endpoint.server, model.FileRequest{Operation: model.FileOperationBackupDelete, Path: job.ArchiveName()})

		var missing *model.FileError
		if err != nil && (!errors.As(err, &missing) || missing.Code != model.FileNotFound) {
			return false, err
		}
	}

	if job.Phase == model.CopyCancelling {
		if targetAbsent {
			return false, s.Store.FinishCancel(ctx, job.ID)
		}

		status, err := s.action(ctx, job.TargetSparkID, model.RuntimeRequest{ServerID: job.TargetServerID, Action: model.ServerActionDelete, OperationID: job.DeleteID})
		if err != nil {
			return false, err
		}

		if status.Busy {
			return true, nil
		}

		if status.State != model.RuntimeStateDeleted || status.Error != "" {
			return false, errors.New("unfinished copy cleanup did not complete")
		}

		return false, s.Store.FinishCancel(ctx, job.ID)
	}

	return false, s.Store.Advance(ctx, job.ID, job.Phase, repository.CopyCheckpoint{Phase: model.CopyComplete})
}
