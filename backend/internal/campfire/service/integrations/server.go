package integrations

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	integrationmodel "campfire/internal/campfire/integrations"
	"campfire/internal/campfire/service/integrations/discord"
	"campfire/internal/shared/model"
	accessperm "campfire/internal/shared/permissions"

	"gorm.io/gorm"
)

// SourceOption keeps data-source authorization independent of the provider.
type SourceOption struct {
	ID         integrationmodel.Category `json:"id"`
	Permission model.PermissionID        `json:"permission"`
}

// NotificationSources is the shared policy used by the API and settings editor.
func NotificationSources() []SourceOption {
	return []SourceOption{
		{integrationmodel.GameEvents, accessperm.ServerView},
		{integrationmodel.PowerEvents, accessperm.ServerView},
		{integrationmodel.ConsoleAlerts, accessperm.ServerConsoleRead},
		{integrationmodel.BackupEvents, accessperm.ServerBackupsRead},
	}
}

// ServerIntegrationInput contains only user-editable binding settings.
type ServerIntegrationInput struct {
	IntegrationID string                          `json:"integration_id"`
	Enabled       bool                            `json:"enabled"`
	ChannelID     string                          `json:"channel_id"`
	Subscriptions []integrationmodel.Subscription `json:"subscriptions"`
	Card          integrationmodel.CardSettings   `json:"card"`
}

// ConnectionOption exposes only the identity needed to select a saved connection.
type ConnectionOption struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
	Name     string `json:"name"`
	Enabled  bool   `json:"enabled"`
}

// ServerIntegrationSettings omits credentials and external message ownership.
type ServerIntegrationSettings struct {
	ServerIntegrationInput
	ID         string                      `json:"id"`
	Configured bool                        `json:"configured"`
	Available  bool                        `json:"available"`
	GuildName  string                      `json:"guild_name"`
	Sources    []SourceOption              `json:"sources"`
	CardStatus integrationmodel.CardStatus `json:"card_status"`
}

// ServerIntegrationList supplies connection availability and independent destinations.
type ServerIntegrationList struct {
	Connections []ConnectionOption          `json:"connections"`
	Configured  bool                        `json:"configured"`
	Available   bool                        `json:"available"`
	GuildName   string                      `json:"guild_name"`
	Sources     []SourceOption              `json:"sources"`
	Items       []ServerIntegrationSettings `json:"items"`
}

func (s *DiscordIntegrationService) requireServerIntegration(ctx context.Context, serverID string, permissions []model.PermissionID) error {
	if !slices.Contains(permissions, accessperm.ServerView) || !slices.Contains(permissions, accessperm.ServerConfigure) {
		return repository.ErrAccessDenied
	}

	var row database.Server

	err := s.db.WithContext(ctx).Select("id").Where("id = ?", serverID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return repository.ErrIntegrationNotFound
	}

	return err
}

func serverBinding(ctx context.Context, store *repository.IntegrationRepository, serverID, bindingID string) (integrationmodel.Binding, integrationmodel.Connection, error) {
	binding, err := store.GetBinding(ctx, bindingID)
	if err != nil {
		return integrationmodel.Binding{}, integrationmodel.Connection{}, err
	}

	if binding.ServerID != serverID {
		return integrationmodel.Binding{}, integrationmodel.Connection{}, repository.ErrIntegrationNotFound
	}

	connection, err := serverConnection(ctx, store, binding.IntegrationID)

	return binding, connection, err
}

func serverConnection(ctx context.Context, store *repository.IntegrationRepository, id string) (integrationmodel.Connection, error) {
	if id == "" {
		// An omitted selection is unambiguous only when a sole connection exists.
		return discordConnection(ctx, store)
	}

	connection, err := store.GetConnection(ctx, id)
	if err != nil {
		return integrationmodel.Connection{}, err
	}

	if connection.Provider != "discord" {
		return integrationmodel.Connection{}, repository.ErrIntegrationNotFound
	}

	return connection, nil
}

func serverSettings(connection integrationmodel.Connection, binding integrationmodel.Binding) ServerIntegrationSettings {
	result := ServerIntegrationSettings{
		ID:                     binding.ID,
		ServerIntegrationInput: ServerIntegrationInput{IntegrationID: connection.ID, Enabled: binding.Enabled, ChannelID: binding.DestinationID, Subscriptions: binding.Subscriptions, Card: binding.Card},
		Configured:             connection.ID != "", Available: connection.ID != "" && connection.Enabled, Sources: NotificationSources(),
	}

	result.CardStatus.State = "disabled"
	if connection.Enabled && binding.Enabled && binding.Card.Enabled {
		result.CardStatus.State = "pending"
	}

	if result.Subscriptions == nil {
		result.Subscriptions = []integrationmodel.Subscription{}
	}

	if result.Card.Fields == nil {
		result.Card.Fields = []string{}
	}

	var config discord.Configuration
	if json.Unmarshal(connection.Config, &config) == nil {
		result.GuildName = config.GuildName
	}

	return result
}

// ServerSettings reads configuration without requiring Discord to be online.
// Grants must be freshly resolved for this server by the panel access service.
func (s *DiscordIntegrationService) ServerSettings(ctx context.Context, serverID, bindingID string, permissions []model.PermissionID) (ServerIntegrationSettings, error) {
	if err := s.requireServerIntegration(ctx, serverID, permissions); err != nil {
		return ServerIntegrationSettings{}, err
	}

	store := repository.NewIntegrationRepository(s.db)

	binding, connection, err := serverBinding(ctx, store, serverID, bindingID)
	if err != nil {
		return ServerIntegrationSettings{}, err
	}

	result := serverSettings(connection, binding)
	result.CardStatus, err = store.CardStatus(ctx, binding, connection)

	return result, err
}

// ListServerIntegrations never contacts Discord and keeps destination IDs server-scoped.
func (s *DiscordIntegrationService) ListServerIntegrations(ctx context.Context, serverID string, permissions []model.PermissionID) (ServerIntegrationList, error) {
	if err := s.requireServerIntegration(ctx, serverID, permissions); err != nil {
		return ServerIntegrationList{}, err
	}

	store := repository.NewIntegrationRepository(s.db)

	connections, err := store.ListConnections(ctx)
	if err != nil {
		return ServerIntegrationList{}, err
	}

	result := ServerIntegrationList{Connections: []ConnectionOption{}, Sources: NotificationSources(), Items: []ServerIntegrationSettings{}}
	byID := make(map[string]integrationmodel.Connection)

	for _, connection := range connections {
		if connection.Provider != "discord" {
			continue
		}

		metadata := serverSettings(connection, integrationmodel.Binding{})

		name := metadata.GuildName
		if name == "" {
			name = connection.Name
		}

		result.Connections = append(result.Connections, ConnectionOption{ID: connection.ID, Provider: connection.Provider, Name: name, Enabled: connection.Enabled})
		byID[connection.ID] = connection
		result.Configured = true
		result.Available = result.Available || connection.Enabled
		result.GuildName = metadata.GuildName
	}

	if len(result.Connections) != 1 {
		result.GuildName = ""
	}

	bindings, err := store.ListBindings(ctx, serverID)
	if err != nil {
		return result, err
	}

	for _, binding := range bindings {
		connection, exists := byID[binding.IntegrationID]
		if !exists {
			continue
		}

		item := serverSettings(connection, binding)

		item.CardStatus, err = store.CardStatus(ctx, binding, connection)
		if err != nil {
			return result, err
		}

		result.Items = append(result.Items, item)
	}

	return result, nil
}

// ServerChannels checks the saved bot without granting global setup access.
func (s *DiscordIntegrationService) ServerChannels(ctx context.Context, serverID, connectionID string, permissions []model.PermissionID) ([]discord.Channel, error) {
	if err := s.requireServerIntegration(ctx, serverID, permissions); err != nil {
		return nil, err
	}

	connection, err := serverConnection(ctx, repository.NewIntegrationRepository(s.db), connectionID)
	if err != nil {
		return nil, err
	}

	return s.bindingChannels(ctx, connection)
}

func (s *DiscordIntegrationService) bindingChannels(ctx context.Context, connection integrationmodel.Connection) ([]discord.Channel, error) {
	if connection.ID == "" || !connection.Enabled {
		return nil, repository.ErrIntegrationDisabled
	}

	var config discord.Configuration
	if json.Unmarshal(connection.Config, &config) != nil {
		return nil, ErrIntegrationInput
	}

	token, err := s.token(connection, "")
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	access, err := s.client.Check(ctx, token, config.GuildID)

	return access.Channels, err
}

func validateServerSettings(input ServerIntegrationInput, permissions []model.PermissionID) error {
	for _, subscription := range input.Subscriptions {
		for _, rule := range subscription.Rules {
			if !(repository.GameEventQuery{Fields: rule.Fields}).ValidFilters() || (rule.Type != "" && !model.ValidGameEventType(rule.Type)) {
				return ErrIntegrationInput
			}
		}

		source := slices.IndexFunc(NotificationSources(), func(source SourceOption) bool { return source.ID == subscription.Category })
		if source < 0 {
			return ErrIntegrationInput
		}

		if input.Enabled && !slices.Contains(permissions, NotificationSources()[source].Permission) {
			return repository.ErrAccessDenied
		}
	}

	if input.Card.Validate() != nil {
		return ErrIntegrationInput
	}

	return nil
}

// SaveServerSettings never deploys game-server configuration. Disabling remains
// possible during a Discord outage, without discarding subscriptions or card identity.
func (s *DiscordIntegrationService) SaveServerSettings(ctx context.Context, serverID, bindingID string, permissions []model.PermissionID, input ServerIntegrationInput) (ServerIntegrationSettings, error) {
	if err := s.requireServerIntegration(ctx, serverID, permissions); err != nil {
		return ServerIntegrationSettings{}, err
	}

	if bindingID == "" {
		bindingID = rand.Text()
	} else if _, _, err := serverBinding(ctx, repository.NewIntegrationRepository(s.db), serverID, bindingID); err != nil {
		return ServerIntegrationSettings{}, err
	}

	values, err := s.SaveServerIntegrationChanges(ctx, serverID, permissions, ServerIntegrationChanges{
		Upserts: []ServerIntegrationChange{{ID: bindingID, Settings: input}},
	})
	if err != nil {
		return ServerIntegrationSettings{}, err
	}

	return values[0], nil
}

// RetryServerCard preserves the creation identity and therefore cannot turn an
// uncertain creation into a replacement send.
func (s *DiscordIntegrationService) RetryServerCard(ctx context.Context, serverID, bindingID string, permissions []model.PermissionID) error {
	if err := s.requireServerIntegration(ctx, serverID, permissions); err != nil {
		return err
	}

	store := repository.NewIntegrationRepository(s.db)

	binding, _, err := serverBinding(ctx, store, serverID, bindingID)
	if err != nil {
		return err
	}

	if binding.ID == "" {
		return repository.ErrIntegrationNotFound
	}

	return store.RetryCard(ctx, binding.ID)
}

// RemoveServerIntegration disconnects only the selected destination, without remote I/O.
// Existing Discord messages remain as history, as with removing the global connection.
func (s *DiscordIntegrationService) RemoveServerIntegration(ctx context.Context, serverID, bindingID string, permissions []model.PermissionID) error {
	_, err := s.SaveServerIntegrationChanges(ctx, serverID, permissions, ServerIntegrationChanges{RemoveIDs: []string{bindingID}})

	return err
}
