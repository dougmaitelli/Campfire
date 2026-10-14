package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"campfire/internal/model"
)

// ErrDeploymentUnavailable means configuration could not reach the assigned node.
var ErrDeploymentUnavailable = errors.New("server configuration delivery unavailable")

// ConfigurationDispatcher delivers desired state through the placement-specific transport.
type ConfigurationDispatcher interface {
	Deploy(context.Context, string, model.Deployment) (model.ConfigurationReceipt, error)
}

// DeploymentService assembles saved configurations and dispatches them to their assigned node.
type DeploymentService struct {
	Servers  *ServerService
	Sparks   *SparkService
	Embers   *EmberService
	Local    ConfigurationDispatcher
	Remote   ConfigurationDispatcher
	mu       sync.Mutex
	locks    map[string]*deliveryLock
	statuses map[string]model.RuntimeStatus
	observed map[string]time.Time
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

	dispatch := s.Remote
	if node.Kind == model.SparkLocal {
		dispatch = s.Local
	}

	if dispatch == nil {
		return model.ConfigurationReceipt{}, ErrDeploymentUnavailable
	}

	receipt, err := dispatch.Deploy(ctx, node.ID, model.Deployment{ServerID: server.ID, Configuration: server.ServerConfiguration, Ember: ember})
	if err != nil {
		return model.ConfigurationReceipt{}, errors.Join(ErrDeploymentUnavailable, err)
	}

	return receipt, nil
}
