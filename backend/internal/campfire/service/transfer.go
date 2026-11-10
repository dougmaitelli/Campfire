package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

// JobDispatcher moves bounded archive chunks over existing Spark sessions.
type JobDispatcher interface {
	Job(context.Context, string, model.JobRequest) (model.JobResult, error)
}

var errTransferCorrupt = errors.New("incoming archive checksum mismatch; retry will copy the archive again")

// TransferService coordinates persistent transfers without staging server data
// on the panel. Both placement reservations remain held until final cleanup.
type TransferService struct {
	Store      *repository.TransferRepository
	Deployment *DeploymentService
}

// NewTransferService creates the durable transfer coordinator.
func NewTransferService(store *repository.TransferRepository, deployment *DeploymentService) *TransferService {
	return &TransferService{Store: store, Deployment: deployment}
}

// Start resumes pending work and dispatches new transfers.
func (s *TransferService) Start(ctx context.Context) {
	s.Deployment.startJobs(ctx, s.Store.Jobs())
}

// Close stops the shared runner and preserves its checkpoints.
func (s *TransferService) Close() {
	s.Deployment.closeJobs()
}

// Begin reserves the destination only after observing a stopped source under
// the same server lock used by power actions and configuration changes.
func (s *TransferService) Begin(ctx context.Context, serverID, targetID string, ports []model.ServerPort) (model.TransferWork, error) {
	if _, ok := s.Deployment.Remote.(JobDispatcher); !ok {
		return model.TransferWork{}, ErrDeploymentUnavailable
	}

	release, err := s.Deployment.lockServer(ctx, serverID)
	if err != nil {
		return model.TransferWork{}, err
	}
	defer release()

	if err := s.Store.Guard(ctx, serverID); err != nil {
		return model.TransferWork{}, err
	}

	server, err := s.Deployment.Servers.Get(ctx, serverID)
	if err != nil {
		return model.TransferWork{}, err
	}

	if server.SparkID == targetID || targetID == "" {
		return model.TransferWork{}, fmt.Errorf("%w: choose a different destination Spark", ErrInvalidServer)
	}

	if server.EmberReviewRequired {
		return model.TransferWork{}, fmt.Errorf("%w: apply the current Ember before transferring", ErrInvalidServer)
	}

	ctx, releaseDefinition, lockErr := holdDefinition(ctx, server.EmberID)
	if lockErr != nil {
		return model.TransferWork{}, lockErr
	}
	defer releaseDefinition()

	ember, err := s.Deployment.Embers.Get(ctx, server.EmberID)
	if err != nil {
		return model.TransferWork{}, err
	}

	if ember.ServerNeedsEmberReview(server) {
		return model.TransferWork{}, fmt.Errorf("%w: apply the current Ember before transferring", ErrInvalidServer)
	}

	if err := s.Deployment.checkJobSparks(ctx, server.SparkID, targetID); err != nil {
		return model.TransferWork{}, err
	}

	if len(ports) != len(server.Ports) {
		return model.TransferWork{}, fmt.Errorf("%w: preserve the game's container port mappings", ErrInvalidServer)
	}

	for i := range ports {
		ip, err := netip.ParseAddr(ports[i].BindIP)
		if err != nil || ip.Zone() != "" || ip.IsMulticast() || ports[i].HostPort < 1 || ports[i].HostPort > 65535 || ports[i].ContainerPort != server.Ports[i].ContainerPort || ports[i].Protocol != server.Ports[i].Protocol {
			return model.TransferWork{}, fmt.Errorf("%w: invalid destination port mapping", ErrInvalidServer)
		}

		ports[i].BindIP = ip.Unmap().String()
	}

	status, err := s.Deployment.runActionUnlocked(ctx, serverID, model.ServerActionStatus, "")
	if err != nil {
		return model.TransferWork{}, err
	}

	if status.Busy || (status.State != model.RuntimeStateStopped && status.State != model.RuntimeStateNotInstalled) {
		return model.TransferWork{}, ErrServerActive
	}

	target := server.ServerConfiguration
	target.SparkID, target.Ports = targetID, ports

	for _, configuration := range []model.ServerConfiguration{server.ServerConfiguration, target} {
		deployment, err := model.NewDeployment(serverID, configuration, ember)
		if err != nil {
			return model.TransferWork{}, err
		}

		if err := s.Deployment.checkQueuedJobCapabilities(deployment, model.JobTransfer); err != nil {
			return model.TransferWork{}, errors.Join(ErrDeploymentUnavailable, err)
		}
	}

	now := time.Now().Unix()

	transfer := model.TransferWork{ID: rand.Text(), ServerID: serverID, SourceSparkID: server.SparkID, TargetSparkID: targetID, SourceConfiguration: server.ServerConfiguration, TargetConfiguration: target, Phase: model.TransferPhaseQueued, Active: true, CreatedAt: now, UpdatedAt: now}
	if err := s.Store.Create(ctx, transfer, server.EmberHash); err != nil {
		return model.TransferWork{}, err
	}

	return transfer, nil
}

func (s *TransferService) step(ctx context.Context, t model.TransferWork) (bool, error) {
	remote, ok := s.Deployment.Remote.(JobDispatcher)
	if !ok {
		return false, ErrDeploymentUnavailable
	}

	call := func(node string, operation model.JobOperation) (model.JobResult, error) {
		return remote.Job(ctx, node, model.JobRequest{ID: t.ID, Attempt: t.Attempt, ServerID: t.ServerID, Operation: operation, Size: t.TotalBytes, Digest: t.Digest})
	}
	next := func(phase model.TransferPhase) (bool, error) {
		return false, s.Store.Advance(ctx, t.ID, t.Phase, map[string]any{"phase": phase, "error": ""})
	}

	switch t.Phase {
	case model.TransferPhaseQueued:
		ember, err := s.Deployment.Embers.Get(ctx, t.SourceConfiguration.EmberID)
		if err != nil {
			return false, err
		}

		deployments := make([]model.Deployment, 0, 2)

		for _, configuration := range []model.ServerConfiguration{t.SourceConfiguration, t.TargetConfiguration} {
			deployment, err := model.NewDeployment(t.ServerID, configuration, ember)
			if err != nil {
				return false, err
			}

			if err := s.Deployment.checkJobCapabilities(deployment, model.JobTransfer); err != nil {
				return false, err
			}

			deployments = append(deployments, deployment)
		}

		for _, deployment := range deployments {
			if _, err := s.Deployment.Remote.Deploy(ctx, deployment.Configuration.SparkID, deployment); err != nil {
				return false, err
			}
		}

		return next(model.TransferPhaseExporting)
	case model.TransferPhaseExporting:
		result, err := call(t.SourceSparkID, model.JobOperationExport)
		if err != nil {
			return false, err
		}

		if result.State == model.JobStateFailed {
			return false, errors.New(result.Error)
		}

		if result.State != model.JobStateReady {
			return true, nil
		}

		return false, s.Store.Advance(ctx, t.ID, t.Phase, map[string]any{"phase": model.TransferPhaseCopying, "total_bytes": result.Size, "digest": result.Digest, "transferred_bytes": 0})
	case model.TransferPhaseCopying:
		result, err := call(t.TargetSparkID, model.JobOperationPrepare)
		if err != nil {
			return false, err
		}

		if result.State == model.JobStateFailed {
			return false, errors.New(result.Error)
		}

		if result.State == model.JobStateImported || result.State == model.JobStateActive {
			return next(model.TransferPhaseImporting)
		}

		if result.State != model.JobStateReceiving || result.Offset > t.TotalBytes {
			return false, errors.New("unexpected destination transfer state")
		}

		if result.Offset == t.TotalBytes {
			return false, s.Store.Advance(ctx, t.ID, t.Phase, map[string]any{"phase": model.TransferPhaseImporting, "transferred_bytes": t.TotalBytes})
		}

		chunk, err := remote.Job(ctx, t.SourceSparkID, model.JobRequest{ID: t.ID, ServerID: t.ServerID, Operation: model.JobOperationRead, Offset: result.Offset})
		if err != nil {
			return false, err
		}

		if len(chunk.Data) == 0 || chunk.Size != t.TotalBytes || chunk.Digest != t.Digest || chunk.Offset != result.Offset+int64(len(chunk.Data)) {
			return false, errors.New("source transfer archive changed")
		}

		ack, err := remote.Job(ctx, t.TargetSparkID, model.JobRequest{ID: t.ID, ServerID: t.ServerID, Operation: model.JobOperationWrite, Offset: result.Offset, Data: chunk.Data})
		if err != nil {
			return false, err
		}

		if ack.Offset != chunk.Offset {
			return false, errors.New("destination transfer offset mismatch")
		}

		return false, s.Store.Advance(ctx, t.ID, t.Phase, map[string]any{"transferred_bytes": ack.Offset})
	case model.TransferPhaseImporting:
		result, err := call(t.TargetSparkID, model.JobOperationImport)
		if err != nil {
			return false, err
		}

		if result.State == model.JobStateCorrupt {
			return false, errTransferCorrupt
		}

		if result.State == model.JobStateFailed {
			return false, errors.New(result.Error)
		}

		if result.State != model.JobStateImported && result.State != model.JobStateActive {
			return true, nil
		}

		return next(model.TransferPhaseCommitting)
	case model.TransferPhaseCommitting:
		return false, s.Store.Commit(ctx, t.ID)
	case model.TransferPhaseCleanup:
		result, err := call(t.TargetSparkID, model.JobOperationActivate)
		if err != nil {
			return false, err
		}

		if result.State != model.JobStateActive {
			return false, errors.New("destination activation incomplete")
		}

		result, err = call(t.SourceSparkID, model.JobOperationCleanup)
		if err != nil {
			return false, err
		}

		if result.State != model.JobStateRetired {
			return true, nil
		}

		s.Deployment.mu.Lock()
		delete(s.Deployment.statuses, t.ServerID)
		delete(s.Deployment.observed, t.ServerID)
		s.Deployment.mu.Unlock()

		return false, s.Store.Advance(ctx, t.ID, t.Phase, map[string]any{"phase": model.TransferPhaseComplete, "active": false, "error": ""})
	case model.TransferPhaseCancelling:
		if t.CancelFrom != model.TransferPhaseQueued && t.CancelFrom != model.TransferPhaseExporting {
			result, err := call(t.TargetSparkID, model.JobOperationDiscard)
			if err != nil {
				return false, err
			}

			if result.State != model.JobStateDiscarded && result.State != model.JobStateAbsent {
				return true, nil
			}
		}

		result, err := call(t.SourceSparkID, model.JobOperationRelease)
		if err != nil {
			return false, err
		}

		if result.State != model.JobStateReleased {
			return true, nil
		}

		return false, s.Store.Advance(ctx, t.ID, t.Phase, map[string]any{"phase": model.TransferPhaseCancelled, "active": false, "error": ""})
	default:
		return false, errors.New("invalid persisted transfer phase")
	}
}
