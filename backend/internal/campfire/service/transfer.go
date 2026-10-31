package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/netip"
	"sync"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

// TransferDispatcher moves bounded archive chunks over existing Spark sessions.
type TransferDispatcher interface {
	Transfer(context.Context, string, model.TransferRequest) (model.TransferResult, error)
}

var errTransferCorrupt = errors.New("incoming archive checksum mismatch; retry will copy the archive again")

// TransferService coordinates persistent transfers without staging server data
// on the panel. Both placement reservations remain held until final cleanup.
type TransferService struct {
	Store      *repository.TransferRepository
	Deployment *DeploymentService
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.Mutex
	running    map[string]bool
	wg         sync.WaitGroup
}

// NewTransferService creates the durable transfer coordinator.
func NewTransferService(store *repository.TransferRepository, deployment *DeploymentService) *TransferService {
	return &TransferService{Store: store, Deployment: deployment, running: make(map[string]bool)}
}

// Start resumes pending work and dispatches new transfers.
func (s *TransferService) Start(ctx context.Context) {
	s.ctx, s.cancel = context.WithCancel(ctx)

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			s.pump()

			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

// Close stops workers without discarding their durable checkpoints.
func (s *TransferService) Close() {
	if s.cancel != nil {
		s.cancel()
		s.wg.Wait()
	}
}

func (s *TransferService) pump() {
	transfers, err := s.Store.List(s.ctx, "", true)
	if err != nil {
		return
	}

	for _, transfer := range transfers {
		s.mu.Lock()
		if len(s.running) >= 4 {
			s.mu.Unlock()
			break
		}

		if !s.running[transfer.ID] {
			s.running[transfer.ID] = true

			s.wg.Add(1)
			go s.run(transfer.ID)
		}
		s.mu.Unlock()
	}
}

// Begin reserves the destination only after observing a stopped source under
// the same server lock used by power actions and configuration changes.
func (s *TransferService) Begin(ctx context.Context, serverID, targetID string, ports []model.ServerPort) (database.ServerTransfer, error) {
	if _, ok := s.Deployment.Remote.(TransferDispatcher); !ok {
		return database.ServerTransfer{}, ErrDeploymentUnavailable
	}

	release, err := s.Deployment.lockServer(ctx, serverID)
	if err != nil {
		return database.ServerTransfer{}, err
	}
	defer release()

	if err := s.Store.Guard(ctx, serverID); err != nil {
		return database.ServerTransfer{}, err
	}

	server, err := s.Deployment.Servers.Get(ctx, serverID)
	if err != nil {
		return database.ServerTransfer{}, err
	}

	if server.SparkID == targetID || targetID == "" {
		return database.ServerTransfer{}, fmt.Errorf("%w: choose a different destination Spark", ErrInvalidServer)
	}

	if server.EmberUpdateAvailable {
		return database.ServerTransfer{}, fmt.Errorf("%w: apply the current Ember before transferring", ErrInvalidServer)
	}

	if checker, ok := s.Deployment.Remote.(interface{ CheckTransfer(string) error }); ok {
		for _, id := range []string{server.SparkID, targetID} {
			if err := checker.CheckTransfer(id); err != nil {
				return database.ServerTransfer{}, errors.Join(ErrDeploymentUnavailable, err)
			}
		}
	}

	if len(ports) != len(server.Ports) {
		return database.ServerTransfer{}, fmt.Errorf("%w: preserve the game's container port mappings", ErrInvalidServer)
	}

	for i := range ports {
		ip, err := netip.ParseAddr(ports[i].BindIP)
		if err != nil || ip.Zone() != "" || ip.IsMulticast() || ports[i].HostPort < 1 || ports[i].HostPort > 65535 || ports[i].ContainerPort != server.Ports[i].ContainerPort || ports[i].Protocol != server.Ports[i].Protocol {
			return database.ServerTransfer{}, fmt.Errorf("%w: invalid destination port mapping", ErrInvalidServer)
		}

		ports[i].BindIP = ip.Unmap().String()
	}

	status, err := s.Deployment.runActionUnlocked(ctx, serverID, "status", "")
	if err != nil {
		return database.ServerTransfer{}, err
	}

	if status.Busy || (status.State != "stopped" && status.State != "not_installed") {
		return database.ServerTransfer{}, ErrServerActive
	}

	target := server.ServerConfiguration
	target.SparkID, target.Ports = targetID, ports
	now := time.Now().Unix()

	transfer := database.ServerTransfer{ID: rand.Text(), ServerID: serverID, SourceSparkID: server.SparkID, TargetSparkID: targetID, SourceConfiguration: server.ServerConfiguration, TargetConfiguration: target, Phase: model.TransferPhaseQueued, Active: true, CanCancel: true, CreatedAt: now, UpdatedAt: now}
	if err := s.Store.Create(ctx, transfer); err != nil {
		return database.ServerTransfer{}, err
	}

	return transfer, nil
}

func (s *TransferService) run(id string) {
	defer s.wg.Done()
	defer func() { s.mu.Lock(); delete(s.running, id); s.mu.Unlock() }()

	ctx, cancel := context.WithTimeout(s.ctx, 45*time.Minute)
	defer cancel()

	for ctx.Err() == nil {
		transfer, err := s.Store.Get(ctx, id)
		if err != nil || !transfer.Active || transfer.Phase == model.TransferPhaseFailed {
			return
		}

		wait, err := s.step(ctx, transfer)
		if errors.Is(err, repository.ErrTransferState) {
			continue
		}

		if err != nil {
			if s.ctx.Err() != nil {
				return
			}

			recordCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)

			resume := transfer.Phase
			if errors.Is(err, errTransferCorrupt) {
				resume = "copying"
			}

			_ = s.Store.Advance(recordCtx, id, transfer.Phase, map[string]any{"phase": model.TransferPhaseFailed, "resume_phase": resume, "error": fmt.Sprintf("%.1024s", err.Error())})

			stop()

			return
		}

		if wait {
			timer := time.NewTimer(250 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
			case <-timer.C:
			}
		}
	}
}

func (s *TransferService) step(ctx context.Context, t database.ServerTransfer) (bool, error) {
	remote, ok := s.Deployment.Remote.(TransferDispatcher)
	if !ok {
		return false, ErrDeploymentUnavailable
	}

	call := func(node string, operation model.TransferOperation) (model.TransferResult, error) {
		return remote.Transfer(ctx, node, model.TransferRequest{ID: t.ID, ServerID: t.ServerID, Operation: operation, Size: t.TotalBytes, Digest: t.Digest})
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

		for _, configuration := range []model.ServerConfiguration{t.SourceConfiguration, t.TargetConfiguration} {
			deployment, err := model.NewDeployment(t.ServerID, configuration, ember)
			if err != nil {
				return false, err
			}

			if _, err := s.Deployment.Remote.Deploy(ctx, configuration.SparkID, deployment); err != nil {
				return false, err
			}
		}

		return next(model.TransferPhaseExporting)
	case model.TransferPhaseExporting:
		result, err := call(t.SourceSparkID, model.TransferOperationExport)
		if err != nil {
			return false, err
		}

		if result.State == model.TransferStateFailed {
			return false, errors.New(result.Error)
		}

		if result.State != model.TransferStateReady {
			return true, nil
		}

		return false, s.Store.Advance(ctx, t.ID, t.Phase, map[string]any{"phase": model.TransferPhaseCopying, "total_bytes": result.Size, "digest": result.Digest, "transferred_bytes": 0})
	case model.TransferPhaseCopying:
		result, err := call(t.TargetSparkID, model.TransferOperationPrepare)
		if err != nil {
			return false, err
		}

		if result.State == model.TransferStateFailed {
			return false, errors.New(result.Error)
		}

		if result.State == model.TransferStateImported || result.State == model.TransferStateActive {
			return next(model.TransferPhaseImporting)
		}

		if result.State != model.TransferStateReceiving || result.Offset > t.TotalBytes {
			return false, errors.New("unexpected destination transfer state")
		}

		if result.Offset == t.TotalBytes {
			return false, s.Store.Advance(ctx, t.ID, t.Phase, map[string]any{"phase": model.TransferPhaseImporting, "transferred_bytes": t.TotalBytes})
		}

		chunk, err := remote.Transfer(ctx, t.SourceSparkID, model.TransferRequest{ID: t.ID, ServerID: t.ServerID, Operation: model.TransferOperationRead, Offset: result.Offset})
		if err != nil {
			return false, err
		}

		if len(chunk.Data) == 0 || chunk.Size != t.TotalBytes || chunk.Digest != t.Digest || chunk.Offset != result.Offset+int64(len(chunk.Data)) {
			return false, errors.New("source transfer archive changed")
		}

		ack, err := remote.Transfer(ctx, t.TargetSparkID, model.TransferRequest{ID: t.ID, ServerID: t.ServerID, Operation: model.TransferOperationWrite, Offset: result.Offset, Data: chunk.Data})
		if err != nil {
			return false, err
		}

		if ack.Offset != chunk.Offset {
			return false, errors.New("destination transfer offset mismatch")
		}

		return false, s.Store.Advance(ctx, t.ID, t.Phase, map[string]any{"transferred_bytes": ack.Offset})
	case model.TransferPhaseImporting:
		result, err := call(t.TargetSparkID, model.TransferOperationImport)
		if err != nil {
			return false, err
		}

		if result.State == model.TransferStateCorrupt {
			return false, errTransferCorrupt
		}

		if result.State == model.TransferStateFailed {
			return false, errors.New(result.Error)
		}

		if result.State != model.TransferStateImported && result.State != model.TransferStateActive {
			return true, nil
		}

		return next(model.TransferPhaseCommitting)
	case model.TransferPhaseCommitting:
		return false, s.Store.Commit(ctx, t.ID)
	case model.TransferPhaseCleanup:
		result, err := call(t.TargetSparkID, model.TransferOperationActivate)
		if err != nil {
			return false, err
		}

		if result.State != model.TransferStateActive {
			return false, errors.New("destination activation incomplete")
		}

		result, err = call(t.SourceSparkID, model.TransferOperationCleanup)
		if err != nil {
			return false, err
		}

		if result.State != model.TransferStateRetired {
			return true, nil
		}

		s.Deployment.mu.Lock()
		delete(s.Deployment.statuses, t.ServerID)
		delete(s.Deployment.observed, t.ServerID)
		s.Deployment.mu.Unlock()

		return false, s.Store.Advance(ctx, t.ID, t.Phase, map[string]any{"phase": model.TransferPhaseComplete, "active": false, "error": ""})
	case model.TransferPhaseCancelling:
		if t.CancelFrom != model.TransferPhaseQueued && t.CancelFrom != model.TransferPhaseExporting {
			result, err := call(t.TargetSparkID, model.TransferOperationDiscard)
			if err != nil {
				return false, err
			}

			if result.State != model.TransferStateDiscarded && result.State != model.TransferStateAbsent {
				return true, nil
			}
		}

		result, err := call(t.SourceSparkID, model.TransferOperationRelease)
		if err != nil {
			return false, err
		}

		if result.State != model.TransferStateReleased {
			return true, nil
		}

		return false, s.Store.Advance(ctx, t.ID, t.Phase, map[string]any{"phase": model.TransferPhaseCancelled, "active": false, "error": ""})
	default:
		return false, errors.New("invalid persisted transfer phase")
	}
}
