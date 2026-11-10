package integrations

import (
	"context"

	"campfire/internal/campfire/database/repository"
	core "campfire/internal/campfire/integrations"
	"campfire/internal/shared/model"
)

// These adapters keep single-destination worker scenarios concise. Production
// calls always use an explicit binding ID; multi-destination tests do so too.
func (s *DiscordIntegrationService) firstTestBinding(ctx context.Context, serverID string) (string, error) {
	bindings, err := repository.NewIntegrationRepository(s.db).ListBindings(ctx, serverID)
	if err != nil || len(bindings) == 0 {
		return "", err
	}

	return bindings[0].ID, nil
}

func (s *DiscordIntegrationService) saveSingleServerSettings(ctx context.Context, serverID string, grants []model.PermissionID, input ServerIntegrationInput) (ServerIntegrationSettings, error) {
	id, err := s.firstTestBinding(ctx, serverID)
	if err != nil {
		return ServerIntegrationSettings{}, err
	}

	return s.SaveServerSettings(ctx, serverID, id, grants, input)
}

func (s *DiscordIntegrationService) singleServerSettings(ctx context.Context, serverID string, grants []model.PermissionID) (ServerIntegrationSettings, error) {
	list, err := s.ListServerIntegrations(ctx, serverID, grants)
	if err != nil {
		return ServerIntegrationSettings{}, err
	}

	if len(list.Items) > 0 {
		return list.Items[0], nil
	}

	return ServerIntegrationSettings{Configured: list.Configured, Available: list.Available, GuildName: list.GuildName, Sources: list.Sources, ServerIntegrationInput: ServerIntegrationInput{Subscriptions: []core.Subscription{}, Card: core.CardSettings{Fields: []string{}}}, CardStatus: core.CardStatus{State: "disabled"}}, nil
}

func (s *DiscordIntegrationService) singleServerMessages(ctx context.Context, serverID string, grants []model.PermissionID, before int64, state string) ([]repository.IntegrationMessageStatus, error) {
	id, err := s.firstTestBinding(ctx, serverID)
	if err != nil {
		return nil, err
	}

	return s.ServerMessages(ctx, serverID, id, grants, before, state)
}

func (s *DiscordIntegrationService) retrySingleServerMessage(ctx context.Context, serverID string, grants []model.PermissionID, id int64) error {
	bindingID, err := s.firstTestBinding(ctx, serverID)
	if err != nil {
		return err
	}

	return s.RetryServerMessage(ctx, serverID, bindingID, grants, id)
}

func (s *DiscordIntegrationService) retrySingleServerCard(ctx context.Context, serverID string, grants []model.PermissionID) error {
	id, err := s.firstTestBinding(ctx, serverID)
	if err != nil {
		return err
	}

	return s.RetryServerCard(ctx, serverID, id, grants)
}
