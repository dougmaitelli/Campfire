package integrations

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"slices"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	core "campfire/internal/campfire/integrations"
	"campfire/internal/campfire/service/integrations/discord"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

// ServerIntegrationChange gives each draft a stable identity across retries.
type ServerIntegrationChange struct {
	ID       string                 `json:"id"`
	Settings ServerIntegrationInput `json:"settings"`
}

// ServerIntegrationChanges applies only the wizard's edits, preserving entries
// added elsewhere since it opened. Repeating a committed batch is safe.
type ServerIntegrationChanges struct {
	Upserts   []ServerIntegrationChange `json:"upserts"`
	RemoveIDs []string                  `json:"remove_ids"`
}

type preparedIntegration struct {
	next       core.Binding
	previous   *core.Binding
	connection core.Connection
}

func bindingSettingsEqual(a, b core.Binding) bool {
	return a.IntegrationID == b.IntegrationID && a.ServerID == b.ServerID &&
		a.DestinationID == b.DestinationID && a.Enabled == b.Enabled &&
		reflect.DeepEqual(a.Subscriptions, b.Subscriptions) && reflect.DeepEqual(a.Card, b.Card)
}

// Existing destinations can stop publishing without requiring Discord access.
// Enabling publishing, changing its destination, or changing its contents still
// checks permissions. Removing subscriptions or disabling the card does not.
func needsPublishingCheck(previous *core.Binding, next core.Binding) bool {
	if !next.Enabled {
		return false
	}

	if previous == nil || !previous.Enabled || previous.IntegrationID != next.IntegrationID || previous.DestinationID != next.DestinationID {
		return true
	}

	if next.Card.Enabled && (!previous.Card.Enabled || !slices.Equal(previous.Card.Fields, next.Card.Fields)) {
		return true
	}

	return slices.ContainsFunc(next.Subscriptions, func(subscription core.Subscription) bool {
		return !slices.ContainsFunc(previous.Subscriptions, func(old core.Subscription) bool { return reflect.DeepEqual(subscription, old) })
	})
}

func (s *DiscordIntegrationService) prepareIntegration(ctx context.Context, serverID string, permissions []model.PermissionID, change ServerIntegrationChange) (preparedIntegration, error) {
	result := preparedIntegration{}
	if err := validateServerSettings(change.Settings, permissions); err != nil {
		return result, err
	}

	store := repository.NewIntegrationRepository(s.db)

	previous, err := store.GetBinding(ctx, change.ID)
	if err == nil {
		if previous.ServerID != serverID {
			return result, repository.ErrIntegrationNotFound
		}

		result.previous = &previous
	} else if !errors.Is(err, repository.ErrIntegrationNotFound) {
		return result, err
	}

	connectionID := change.Settings.IntegrationID
	if result.previous != nil {
		if connectionID != "" && connectionID != previous.IntegrationID {
			return result, ErrIntegrationInput
		}

		connectionID = previous.IntegrationID
	}

	connection, err := serverConnection(ctx, store, connectionID)
	if err != nil {
		return result, err
	}

	input := change.Settings

	result.next = core.Binding{ID: change.ID, IntegrationID: connection.ID, ServerID: serverID, DestinationID: input.ChannelID, Enabled: input.Enabled, Subscriptions: input.Subscriptions, Card: input.Card}
	if result.next.Subscriptions == nil {
		result.next.Subscriptions = []core.Subscription{}
	}

	if connection.ID == "" || !discord.ValidID(input.ChannelID) || result.next.Validate() != nil {
		return result, ErrIntegrationInput
	}

	result.connection = connection

	return result, nil
}

// SaveServerIntegrationChanges validates remote publishing access before taking
// the write lock, then commits every integration edit or none of them. Stable
// IDs and idempotent removals allow recovery after an ambiguous HTTP response.
func (s *DiscordIntegrationService) SaveServerIntegrationChanges(ctx context.Context, serverID string, permissions []model.PermissionID, input ServerIntegrationChanges) ([]ServerIntegrationSettings, error) {
	if err := s.requireServerIntegration(ctx, serverID, permissions); err != nil {
		return nil, err
	}

	if len(input.Upserts)+len(input.RemoveIDs) > 128 {
		return nil, ErrIntegrationInput
	}

	seen := map[string]bool{}
	for _, id := range input.RemoveIDs {
		if id == "" || len(id) > 256 || seen[id] {
			return nil, ErrIntegrationInput
		}

		seen[id] = true
	}

	prepared := make([]preparedIntegration, 0, len(input.Upserts))
	checked := map[string][]discord.Channel{}

	for _, change := range input.Upserts {
		if seen[change.ID] {
			return nil, ErrIntegrationInput
		}

		seen[change.ID] = true

		item, err := s.prepareIntegration(ctx, serverID, permissions, change)
		if err != nil {
			return nil, err
		}

		if needsPublishingCheck(item.previous, item.next) {
			channels, exists := checked[item.connection.ID]
			if !exists {
				channels, err = s.bindingChannels(ctx, item.connection)
				if err != nil {
					return nil, err
				}

				checked[item.connection.ID] = channels
			}

			if !slices.ContainsFunc(channels, func(channel discord.Channel) bool { return channel.ID == item.next.DestinationID && channel.Available }) {
				return nil, &discord.Error{Message: "Choose a channel in the configured Discord server with all required bot permissions."}
			}
		}

		prepared = append(prepared, item)
	}

	err := database.Transaction(s.db.WithContext(ctx), func(tx *gorm.DB) error {
		store := repository.NewIntegrationRepository(tx)
		for _, item := range prepared {
			connection, err := serverConnection(ctx, store, item.connection.ID)
			if err != nil {
				return err
			}

			if connection.Enabled != item.connection.Enabled || connection.CredentialRef != item.connection.CredentialRef || !bytes.Equal(connection.Config, item.connection.Config) {
				return repository.ErrIntegrationConflict
			}

			current, err := store.GetBinding(ctx, item.next.ID)
			if item.previous == nil {
				if !errors.Is(err, repository.ErrIntegrationNotFound) {
					if err != nil {
						return err
					}

					return repository.ErrIntegrationConflict
				}
			} else if err != nil {
				return err
			} else if !bindingSettingsEqual(current, *item.previous) {
				return repository.ErrIntegrationConflict
			}
		}

		for _, id := range input.RemoveIDs {
			binding, err := store.GetBinding(ctx, id)
			if errors.Is(err, repository.ErrIntegrationNotFound) {
				continue
			} else if err != nil {
				return err
			}

			if binding.ServerID != serverID {
				return repository.ErrIntegrationNotFound
			}

			if _, err := serverConnection(ctx, store, binding.IntegrationID); err != nil {
				return err
			}

			if err := store.DeleteBinding(ctx, id); err != nil {
				return err
			}
		}
		// Vacate moving destinations only within this transaction. Discord IDs are
		// numeric, so this reserved namespace cannot collide with a real channel.
		// Worker references remain unchanged and readers never see these values.
		for _, item := range prepared {
			if item.previous != nil && item.previous.DestinationID != item.next.DestinationID {
				if err := tx.Model(&database.ServerIntegration{}).Where("id = ? AND server_id = ?", item.next.ID, serverID).
					Update("destination_id", "moving:"+item.next.ID).Error; err != nil {
					return err
				}
			}
		}

		for _, item := range prepared {
			var err error
			if item.previous == nil {
				err = store.CreateBinding(ctx, item.next)
			} else {
				err = store.UpdateBinding(ctx, item.next)
			}

			if err != nil {
				return err
			}

			if item.next.Enabled {
				if err := store.ResumeMessages(ctx, item.next.IntegrationID, item.next.DestinationID); err != nil {
					return err
				}
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	result := make([]ServerIntegrationSettings, 0, len(prepared))
	for _, item := range prepared {
		settings, err := s.ServerSettings(ctx, serverID, item.next.ID, permissions)
		if err != nil {
			return nil, err
		}

		result = append(result, settings)
	}

	return result, nil
}
