package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

// ErrDeploymentUnavailable means configuration could not reach the assigned node.
var ErrDeploymentUnavailable = errors.New("server configuration delivery unavailable")

// ConfigurationDispatcher delivers desired state through the placement-specific transport.
type ConfigurationDispatcher interface {
	Deploy(context.Context, string, model.Deployment) (model.ConfigurationReceipt, error)
}

// DeploymentService assembles saved configurations and dispatches them to their assigned node.
type DeploymentService struct {
	MaxUploadBytes int64
	Servers        *ServerService
	Sparks         *SparkService
	Embers         *EmberService
	Remote         ConfigurationDispatcher
	Activity       *ActivityService
	Transfers      *TransferService
	Copies         *CopyService
	Jobs           *JobService
	Backups        *BackupService
	History        *ResourceHistoryService
	Games          *GameIntegrationService
	LogAlerts      *LogAlertService
	mu             sync.Mutex
	locks          map[string]*deliveryLock
	statuses       map[string]model.RuntimeStatus
	observed       map[string]time.Time
	statusSparks   map[string]string
	delivered      map[string]string
}

func (s *DeploymentService) guardServer(ctx context.Context, id string) error {
	if s.Copies != nil {
		return s.Copies.Store.Guard(ctx, id)
	}

	if s.Transfers != nil {
		return s.Transfers.Store.Guard(ctx, id)
	}

	return nil
}

type deliveryLock struct {
	gate  chan struct{}
	users int
}

// Sync delivers the saved definition without installing or starting a server.
func (s *DeploymentService) Sync(ctx context.Context, id string) (model.ConfigurationReceipt, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	release, err := s.lockServer(ctx, id)
	if err != nil {
		return model.ConfigurationReceipt{}, err
	}
	defer release()

	return s.sync(ctx, id)
}

func (s *DeploymentService) sync(ctx context.Context, id string) (model.ConfigurationReceipt, error) {
	return s.deliverConfiguration(ctx, id, false)
}

// syncChanged reconciles saved edits independently of read-only status requests.
func (s *DeploymentService) syncChanged(ctx context.Context, id string) error {
	release, err := s.lockServer(ctx, id)
	if err != nil {
		return err
	}
	defer release()

	if err := s.guardServer(ctx, id); err != nil {
		if errors.Is(err, repository.ErrJobActive) {
			return nil
		}

		return err
	}

	server, err := s.Servers.Get(ctx, id)
	if err != nil {
		return err
	}

	if server.EmberReviewRequired {
		return nil
	}

	_, err = s.deliverConfiguration(ctx, id, true)

	return err
}

func (s *DeploymentService) deliverConfiguration(ctx context.Context, id string, changedOnly bool) (model.ConfigurationReceipt, error) {
	if err := s.guardServer(ctx, id); err != nil {
		return model.ConfigurationReceipt{}, err
	}

	if err := ctx.Err(); err != nil {
		return model.ConfigurationReceipt{}, err
	}

	server, err := s.Servers.Get(ctx, id)
	if err != nil {
		return model.ConfigurationReceipt{}, err
	}

	node, err := s.Sparks.Get(ctx, server.SparkID)
	if err != nil {
		return model.ConfigurationReceipt{}, err
	}

	if node.Disabled {
		return model.ConfigurationReceipt{}, ErrDeploymentUnavailable
	}

	ctx, releaseDefinition, lockErr := holdDefinition(ctx, server.EmberID)
	if lockErr != nil {
		return model.ConfigurationReceipt{}, lockErr
	}
	defer releaseDefinition()

	ember, err := s.Embers.Get(ctx, server.EmberID)
	if err != nil {
		return model.ConfigurationReceipt{}, err
	}

	if ember.ServerNeedsEmberReview(server) {
		return model.ConfigurationReceipt{}, fmt.Errorf("%w: review server values before synchronizing configuration", ErrInvalidServer)
	}

	dispatch := s.Remote

	if dispatch == nil {
		return model.ConfigurationReceipt{}, ErrDeploymentUnavailable
	}

	deployment, err := model.NewDeployment(server.ID, server.ServerConfiguration, ember)
	if err != nil {
		return model.ConfigurationReceipt{}, errors.Join(ErrInvalidServer, err)
	}

	revision := model.DeploymentHash(deployment)

	s.mu.Lock()
	delivered := s.delivered[id] == revision
	s.mu.Unlock()

	if changedOnly && delivered {
		if server.EmberHash != revision {
			if err := s.Servers.store.AcknowledgeConfiguration(ctx, server, revision); err != nil {
				return model.ConfigurationReceipt{}, err
			}
		}

		return model.ConfigurationReceipt{ServerID: id, Revision: revision}, nil
	}

	receipt, err := dispatch.Deploy(ctx, node.ID, deployment)
	if err != nil {
		return model.ConfigurationReceipt{}, errors.Join(ErrDeploymentUnavailable, err)
	}

	if receipt.ServerID != id || receipt.Revision != revision {
		return model.ConfigurationReceipt{}, fmt.Errorf("%w: Spark returned an invalid configuration receipt", ErrDeploymentUnavailable)
	}

	s.mu.Lock()
	if s.delivered == nil {
		s.delivered = make(map[string]string)
	}

	s.delivered[id] = revision
	s.mu.Unlock()

	if server.EmberHash != revision {
		if err := s.Servers.store.AcknowledgeConfiguration(ctx, server, revision); err != nil {
			return model.ConfigurationReceipt{}, err
		}
	}

	return receipt, nil
}
