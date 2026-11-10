package integrations

import (
	"context"
	"slices"
	"time"

	"campfire/internal/campfire/database/repository"
	core "campfire/internal/campfire/integrations"
	"campfire/internal/shared/model"
)

func messageCategories(permissions []model.PermissionID) []core.Category {
	categories := []core.Category{}

	for _, source := range NotificationSources() {
		if slices.Contains(permissions, source.Permission) {
			categories = append(categories, source.ID)
		}
	}

	return categories
}

// ServerMessages limits status visibility to the caller's current source grants.
func (s *DiscordIntegrationService) ServerMessages(ctx context.Context, serverID, bindingID string, permissions []model.PermissionID, before int64, state string) ([]repository.IntegrationMessageStatus, error) {
	if err := s.requireServerIntegration(ctx, serverID, permissions); err != nil {
		return nil, err
	}

	if before < 0 || !slices.Contains([]string{"", "pending", "delivered", "failed", "uncertain"}, state) {
		return nil, ErrIntegrationInput
	}

	store := repository.NewIntegrationRepository(s.db)

	binding, _, err := serverBinding(ctx, store, serverID, bindingID)
	if err != nil {
		return nil, err
	}

	return store.MessageStatuses(ctx, binding.ID, messageCategories(permissions), before, state)
}

// RetryServerMessage requeues known failures or requests receipt-only recovery.
func (s *DiscordIntegrationService) RetryServerMessage(ctx context.Context, serverID, bindingID string, permissions []model.PermissionID, id int64) error {
	if err := s.requireServerIntegration(ctx, serverID, permissions); err != nil {
		return err
	}

	store := repository.NewIntegrationRepository(s.db)

	binding, connection, err := serverBinding(ctx, store, serverID, bindingID)
	if err != nil {
		return err
	}

	if !connection.Enabled || !binding.Enabled {
		return repository.ErrIntegrationDisabled
	}

	return store.RetryMessage(ctx, binding.ID, id, messageCategories(permissions), time.Now())
}
