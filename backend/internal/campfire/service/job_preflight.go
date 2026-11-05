package service

import (
	"context"

	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
)

func (s *DeploymentService) checkJobCapabilities(deployment model.Deployment, kind model.JobType) error {
	if checker, ok := s.Remote.(interface{ CheckCapabilities(string, ...string) error }); ok {
		required := protocol.RequiredCapabilities(protocol.Command{Name: protocol.ConfigureServer, Deployment: &deployment})

		required = append(required, protocol.CapabilityLifecycle, protocol.CapabilityJobs)
		if kind == model.JobCopy {
			required = append(required, protocol.CapabilityFiles, protocol.CapabilityBackups)
		}

		return checker.CheckCapabilities(deployment.Configuration.SparkID, required...)
	}

	return nil
}

// Disconnected destinations can reserve a queued job. Live feature checks run
// again during preparation, before either endpoint receives configuration.
func (s *DeploymentService) checkQueuedJobCapabilities(deployment model.Deployment, kind model.JobType) error {
	id := deployment.Configuration.SparkID
	if s.Sparks.connectivity == nil || !s.Sparks.connectivity.Connected(id) {
		return nil
	}

	err := s.checkJobCapabilities(deployment, kind)
	if err != nil && !s.Sparks.connectivity.Connected(id) {
		return nil
	}

	return err
}

func (s *DeploymentService) checkJobSparks(ctx context.Context, ids ...string) error {
	for _, id := range ids {
		spark, err := s.Sparks.Get(ctx, id)
		if err != nil {
			return err
		}

		if spark.Disabled {
			return ErrDeploymentUnavailable
		}
	}

	return nil
}
