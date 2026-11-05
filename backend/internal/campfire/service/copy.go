package service

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"time"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

// CopyService coordinates existing backup operations without moving the source.
type CopyService struct {
	Store      *repository.CopyRepository
	Deployment *DeploymentService
}

// NewCopyService constructs the persistent, bounded copy coordinator.
func NewCopyService(store *repository.CopyRepository, deployment *DeploymentService) *CopyService {
	return &CopyService{Store: store, Deployment: deployment}
}

// Start resumes pending copies independently of browser connections.
func (s *CopyService) Start(ctx context.Context) {
	s.Deployment.startJobs(ctx, s.Store.Jobs())
}

// Close stops the shared runner and preserves its checkpoints.
func (s *CopyService) Close() {
	s.Deployment.closeJobs()
}

// Create copies hidden settings on the backend and reserves a new identity.
func (s *CopyService) Create(ctx context.Context, sourceID string, input model.ServerConfiguration, ownerID int64, quick, files bool, policy *model.BackupPolicy) (model.Server, error) {
	release, err := s.Deployment.lockServer(ctx, sourceID)
	if err != nil {
		return model.Server{}, err
	}
	defer release()

	if err := s.Store.Guard(ctx, sourceID); err != nil {
		return model.Server{}, err
	}

	source, err := s.Deployment.Servers.Get(ctx, sourceID)
	if err != nil {
		return model.Server{}, err
	}

	if source.EmberUpdateAvailable {
		return model.Server{}, fmt.Errorf("%w: apply the current Ember before copying", ErrInvalidServer)
	}

	ctx, releaseDefinition, lockErr := holdDefinition(ctx, source.EmberID)
	if lockErr != nil {
		return model.Server{}, lockErr
	}
	defer releaseDefinition()

	ember, err := s.Deployment.Embers.Get(ctx, source.EmberID)
	if err != nil {
		return model.Server{}, err
	}

	if source.EmberHash != ember.Revision {
		return model.Server{}, fmt.Errorf("%w: apply the current Ember before copying", ErrInvalidServer)
	}

	if quick {
		targetSpark := input.SparkID
		input = source.ServerConfiguration
		input.SparkID = targetSpark
		files = true
	} else if input.EmberID != source.EmberID {
		return model.Server{}, fmt.Errorf("%w: preserve the source Ember when copying", ErrInvalidServer)
	}

	input.Variables = maps.Clone(input.Variables)
	if input.Variables == nil {
		input.Variables = make(map[string]string)
	}

	for _, variable := range ember.Variables {
		if !variable.Hidden {
			continue
		}

		if !quick {
			if _, supplied := input.Variables[variable.Name]; supplied {
				return model.Server{}, fmt.Errorf("%w: hidden variables cannot be supplied when copying", ErrInvalidServer)
			}
		}

		if value, exists := source.Variables[variable.Name]; exists {
			input.Variables[variable.Name] = value
		}
	}

	if policy != nil {
		if s.Deployment.Backups == nil {
			return model.Server{}, ErrDeploymentUnavailable
		}

		if err := s.Deployment.Backups.validatePolicy(*policy); err != nil {
			return model.Server{}, err
		}
	}

	if files {
		if _, ok := s.Deployment.Remote.(RuntimeDispatcher); !ok {
			return model.Server{}, ErrDeploymentUnavailable
		}

		if _, ok := s.Deployment.Remote.(FileDispatcher); !ok {
			return model.Server{}, ErrDeploymentUnavailable
		}

		if err := s.Deployment.checkJobSparks(ctx, source.SparkID, input.SparkID); err != nil {
			return model.Server{}, err
		}

		status, err := s.Deployment.runActionUnlocked(ctx, source.ID, model.ServerActionStatus, "")
		if err != nil {
			return model.Server{}, err
		}

		if status.Busy || (status.State != model.RuntimeStateRunning && status.State != model.RuntimeStateStopped) {
			return model.Server{}, fmt.Errorf("%w: wait until the source is installed and no other operation is in progress", repository.ErrJobState)
		}

		if _, ok := s.Deployment.Remote.(JobDispatcher); !ok {
			return model.Server{}, ErrDeploymentUnavailable
		}
	}

	return s.Store.Create(ctx, source, ember, input, quick, files, policy, func(store *repository.ServerRepository, emberStore *repository.EmberRepository, configuration model.ServerConfiguration) (model.Server, error) {
		target, err := NewServerService(store, NewEmberService(emberStore)).CreateOwned(ctx, configuration, ownerID)
		if err != nil {
			return model.Server{}, err
		}

		if files {
			for _, server := range []model.Server{source, target} {
				deployment, err := model.NewDeployment(server.ID, server.ServerConfiguration, ember)
				if err != nil {
					return model.Server{}, err
				}

				if err := s.Deployment.checkQueuedJobCapabilities(deployment, model.JobCopy); err != nil {
					return model.Server{}, errors.Join(ErrDeploymentUnavailable, err)
				}
			}
		}

		return target, nil
	})
}

func (s *CopyService) files(ctx context.Context, node, server string, request model.FileRequest) (model.FileResult, error) {
	remote, ok := s.Deployment.Remote.(FileDispatcher)
	if !ok {
		return model.FileResult{}, ErrDeploymentUnavailable
	}

	request.ServerID, request.UploadLimit = server, s.Deployment.UploadLimit()

	call, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	return remote.Files(call, node, request)
}

func (s *CopyService) action(ctx context.Context, node string, request model.RuntimeRequest) (model.RuntimeStatus, error) {
	remote, ok := s.Deployment.Remote.(RuntimeDispatcher)
	if !ok {
		return model.RuntimeStatus{}, ErrDeploymentUnavailable
	}

	call, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	return remote.Action(call, node, request)
}

func (s *CopyService) archive(ctx context.Context, job model.CopyWork) (*model.FileEntry, error) {
	result, err := s.files(ctx, job.SourceSparkID, job.SourceServerID, model.FileRequest{Operation: model.FileOperationBackupList, Path: "."})
	if err != nil {
		return nil, err
	}

	for _, entry := range result.Entries {
		if entry.Name == job.ArchiveName() {
			if !entry.Portable || entry.BackupMode != model.BackupModeFull || entry.Size < 1 || entry.Size > s.Deployment.UploadLimit() {
				return nil, errors.New("copy backup is invalid or exceeds the configured upload limit")
			}

			return &entry, nil
		}
	}

	return nil, nil
}
