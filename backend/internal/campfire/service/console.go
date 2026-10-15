package service

import (
	"context"
	"errors"

	"campfire/internal/shared/model"
)

// ConsoleDispatcher streams output over the authenticated Spark connection.
type ConsoleDispatcher interface {
	Console(context.Context, string, string, func(model.ConsoleEvent) error) error
}

// StreamConsole reads saved placement without deploying or locking lifecycle operations.
func (s *DeploymentService) StreamConsole(ctx context.Context, id string, emit func(model.ConsoleEvent) error) error {
	server, err := s.Servers.Get(ctx, id)
	if err != nil {
		return err
	}

	node, err := s.Sparks.Get(ctx, server.SparkID)
	if err != nil {
		return err
	}

	if node.Disabled {
		return ErrDeploymentUnavailable
	}

	remote, ok := s.Remote.(ConsoleDispatcher)
	if !ok {
		return ErrDeploymentUnavailable
	}

	if err := remote.Console(ctx, node.ID, id, emit); err != nil {
		return errors.Join(ErrDeploymentUnavailable, err)
	}

	return nil
}
