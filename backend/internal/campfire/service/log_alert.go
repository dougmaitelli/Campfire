package service

import (
	"context"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

// LogAlertService ingests the authenticated Spark's durable observations.
type LogAlertService struct {
	Store         *repository.LogAlertRepository
	Notifications *NotificationService
}

// Observe retains detected issues and queues configured notification delivery.
func (s *LogAlertService) Observe(ctx context.Context, sparkID, serverID string, observation *model.LogObservation) error {
	if observation == nil {
		return nil
	}

	queue := s.Notifications != nil && s.Notifications.Apprise != nil

	return s.Store.Observe(ctx, sparkID, serverID, *observation, queue)
}
