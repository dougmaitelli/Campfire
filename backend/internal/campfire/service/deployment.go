package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

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
	Backups        *BackupService
	History        *ResourceHistoryService
	Games          *GameIntegrationService
	mu             sync.Mutex
	locks          map[string]*deliveryLock
	statuses       map[string]model.RuntimeStatus
	observed       map[string]time.Time
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
	if s.Transfers != nil {
		if err := s.Transfers.Store.Guard(ctx, id); err != nil {
			return model.ConfigurationReceipt{}, err
		}
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

	ember, err := s.Embers.Get(ctx, server.EmberID)
	if err != nil {
		return model.ConfigurationReceipt{}, err
	}

	if server.EmberUpdateAvailable {
		return model.ConfigurationReceipt{}, fmt.Errorf("%w: apply the current Ember before synchronizing configuration", ErrInvalidServer)
	}

	dispatch := s.Remote

	if dispatch == nil {
		return model.ConfigurationReceipt{}, ErrDeploymentUnavailable
	}

	receipt, err := dispatch.Deploy(ctx, node.ID, model.Deployment{ServerID: server.ID, Configuration: server.ServerConfiguration, Ember: ember})
	if err != nil {
		return model.ConfigurationReceipt{}, errors.Join(ErrDeploymentUnavailable, err)
	}

	return receipt, nil
}
