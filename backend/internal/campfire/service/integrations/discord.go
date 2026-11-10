// Package integrations coordinates external integrations independently of server deployment.
package integrations

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	integrationmodel "campfire/internal/campfire/integrations"
	"campfire/internal/campfire/service/integrations/discord"

	"gorm.io/gorm"
)

// ErrIntegrationInput reports safe, actionable setup errors.
var ErrIntegrationInput = errors.New("invalid integration settings")

// Principal provides the authorization boundary without importing the parent service package.
type Principal interface{ CanManageIntegrations() bool }

// DiscordIntegrationService manages the initial singleton Discord connection.
type DiscordIntegrationService struct {
	db       *gorm.DB
	secrets  Secrets
	client   *discord.Client
	delivery *DeliveryWorker
	cards    *CardWorker
}

// DiscordIntegrationInput leaves a blank token unchanged on an existing connection.
type DiscordIntegrationInput struct {
	Enabled bool   `json:"enabled"`
	GuildID string `json:"guild_id"`
	Token   string `json:"token"`
}

// DiscordIntegrationStatus contains only public configuration and checked health.
type DiscordIntegrationStatus struct {
	InviteURL           string                `json:"invite_url"`
	Configured          bool                  `json:"configured"`
	Enabled             bool                  `json:"enabled"`
	Healthy             bool                  `json:"healthy"`
	Configuration       discord.Configuration `json:"configuration"`
	Channels            []discord.Channel     `json:"channels"`
	RequiredPermissions []string              `json:"required_permissions"`
	CheckedAt           int64                 `json:"checked_at"`
	Error               string                `json:"error"`
}

// NewDiscordIntegrationService keeps provider transport separate from persistence.
func NewDiscordIntegrationService(db *gorm.DB, secrets Secrets, client *discord.Client) *DiscordIntegrationService {
	provider := &discordProvider{secrets: secrets, client: client, media: repository.NewGameMediaRepository(db)}
	store := repository.NewIntegrationRepository(db)

	return &DiscordIntegrationService{db: db, secrets: secrets, client: client, delivery: &DeliveryWorker{Store: store, Providers: map[string]MessageProvider{"discord": provider}}, cards: &CardWorker{Store: store, Providers: map[string]CardProvider{"discord": provider}}}
}

// WithCards connects a provider-independent source before workers start.
func (s *DiscordIntegrationService) WithCards(source CardSource) *DiscordIntegrationService {
	s.cards.Source = source
	return s
}

// Start enables notification delivery and optional server-card reconciliation.
func (s *DiscordIntegrationService) Start(ctx context.Context) {
	s.delivery.Start(ctx)

	if s.cards.Source != nil {
		s.cards.Start(ctx)
	}
}

// Close stops notification delivery without changing saved bindings.
func (s *DiscordIntegrationService) Close() { s.delivery.Close(); s.cards.Close() }

func discordConnection(ctx context.Context, store *repository.IntegrationRepository) (integrationmodel.Connection, error) {
	connections, err := store.ListConnections(ctx)
	if err != nil {
		return integrationmodel.Connection{}, err
	}

	var result integrationmodel.Connection

	for _, item := range connections {
		if item.Provider != "discord" {
			continue
		}

		if result.ID != "" {
			return result, ErrIntegrationInput
		}

		result = item
	}

	return result, nil
}

func (s *DiscordIntegrationService) connection(ctx context.Context, p Principal) (integrationmodel.Connection, error) {
	if !p.CanManageIntegrations() {
		return integrationmodel.Connection{}, repository.ErrAccessDenied
	}

	return discordConnection(ctx, repository.NewIntegrationRepository(s.db))
}

func (s *DiscordIntegrationService) token(connection integrationmodel.Connection, supplied string) (string, error) {
	if token := strings.TrimSpace(supplied); token != "" {
		return token, nil
	}

	if connection.CredentialRef == "" {
		return "", ErrIntegrationInput
	}

	return s.secrets.Get(connection.CredentialRef)
}

func discordStatus(connection integrationmodel.Connection, access *discord.Access) DiscordIntegrationStatus {
	result := DiscordIntegrationStatus{Configured: connection.ID != "", Enabled: connection.Enabled, Channels: []discord.Channel{}, RequiredPermissions: discord.RequiredPermissionNames()}
	if connection.ID != "" {
		if json.Unmarshal(connection.Config, &result.Configuration) != nil {
			result.Error = "Saved Discord settings are invalid. Save the connection again."
		}
	}

	if access != nil {
		result.Configuration = discord.Configuration{BotID: access.Bot.ID, BotName: access.Bot.Username, GuildID: access.Guild.ID, GuildName: access.Guild.Name}
		result.Channels, result.CheckedAt = access.Channels, time.Now().Unix()

		result.Healthy = connection.Enabled && slices.ContainsFunc(access.Channels, func(item discord.Channel) bool { return item.Available })
		if connection.Enabled && !result.Healthy {
			result.Error = "No text channel has all required bot permissions. Update the bot's Discord permissions."
		}
	}

	result.InviteURL = discord.BotInviteURL(result.Configuration.BotID, result.Configuration.GuildID)

	return result
}

// Status checks saved credentials without exposing them or replacing configuration.
func (s *DiscordIntegrationService) Status(ctx context.Context, p Principal) (DiscordIntegrationStatus, error) {
	connection, err := s.connection(ctx, p)
	if err != nil {
		return DiscordIntegrationStatus{}, err
	}

	result := discordStatus(connection, nil)
	if !result.Configured || !result.Enabled || result.Error != "" {
		return result, nil
	}

	token, err := s.token(connection, "")
	if err != nil {
		result.Error = err.Error()
		return result, nil
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	access, err := s.client.Check(ctx, token, result.Configuration.GuildID)
	if err != nil {
		result.CheckedAt, result.Error = time.Now().Unix(), err.Error()
		return result, nil
	}

	return discordStatus(connection, &access), nil
}

// Preview validates a new or saved token before offering its Discord servers.
func (s *DiscordIntegrationService) Preview(ctx context.Context, p Principal, supplied string) (discord.Preview, error) {
	connection, err := s.connection(ctx, p)
	if err != nil {
		return discord.Preview{}, err
	}

	token, err := s.token(connection, supplied)
	if err != nil {
		return discord.Preview{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	return s.client.Preview(ctx, token)
}

// Save checks remote access before serializing local changes. Disabling an
// unchanged connection works even while Discord or its credentials are unavailable.
func (s *DiscordIntegrationService) Save(ctx context.Context, p Principal, input DiscordIntegrationInput) (DiscordIntegrationStatus, error) {
	previous, err := s.connection(ctx, p)
	if err != nil {
		return DiscordIntegrationStatus{}, err
	}

	if !discord.ValidID(input.GuildID) {
		return DiscordIntegrationStatus{}, ErrIntegrationInput
	}

	var oldConfig discord.Configuration
	if previous.ID != "" && json.Unmarshal(previous.Config, &oldConfig) != nil {
		return DiscordIntegrationStatus{}, ErrIntegrationInput
	}

	var access *discord.Access

	var token string
	if input.Enabled || previous.ID == "" || strings.TrimSpace(input.Token) != "" || input.GuildID != oldConfig.GuildID {
		token, err = s.token(previous, input.Token)
		if err != nil {
			return DiscordIntegrationStatus{}, err
		}

		checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		checked, checkErr := s.client.Check(checkCtx, token, input.GuildID)

		cancel()

		if checkErr != nil {
			return DiscordIntegrationStatus{}, checkErr
		}

		access = &checked
	}

	next := previous
	if next.ID == "" {
		next.ID, next.Provider = rand.Text(), "discord"
	}

	next.Enabled = input.Enabled
	if access != nil {
		next.Name = "Discord"

		next.Config, err = json.Marshal(discord.Configuration{BotID: access.Bot.ID, BotName: access.Bot.Username, GuildID: access.Guild.ID, GuildName: access.Guild.Name})
		if err != nil {
			return DiscordIntegrationStatus{}, err
		}
	}

	createdSecret := ""
	if previous.ID == "" || strings.TrimSpace(input.Token) != "" {
		createdSecret, err = s.secrets.Put(token)
		if err != nil {
			return DiscordIntegrationStatus{}, err
		}

		next.CredentialRef = createdSecret
	}

	err = database.Transaction(s.db.WithContext(ctx), func(tx *gorm.DB) error {
		store := repository.NewIntegrationRepository(tx)

		current, err := discordConnection(ctx, store)
		if err != nil {
			return err
		}

		if current.ID != previous.ID || current.Enabled != previous.Enabled || current.CredentialRef != previous.CredentialRef || !bytes.Equal(current.Config, previous.Config) {
			return repository.ErrIntegrationConflict
		}

		if previous.ID != "" && access != nil && (oldConfig.GuildID != access.Guild.ID || oldConfig.BotID != access.Bot.ID) {
			var bindings int64
			if err := tx.Model(&database.ServerIntegration{}).Where("integration_id = ?", previous.ID).Count(&bindings).Error; err != nil {
				return err
			}

			if bindings != 0 {
				return repository.ErrIntegrationConflict
			}
		}

		if current.ID == "" {
			return store.CreateConnection(ctx, next)
		}

		if err := store.UpdateConnection(ctx, next); err != nil {
			return err
		}

		if next.Enabled {
			return store.ResumeMessages(ctx, next.ID, "")
		}

		return nil
	})
	if err != nil {
		if createdSecret != "" {
			_ = s.secrets.Delete(createdSecret)
		}

		return DiscordIntegrationStatus{}, err
	}

	if createdSecret != "" && previous.CredentialRef != "" {
		if err := s.secrets.Delete(previous.CredentialRef); err != nil {
			slog.Warn("retire integration credential", "error", err)
		}
	}

	return discordStatus(next, access), nil
}

// Remove disconnects all bindings locally, including while Discord is unavailable.
// Sent messages remain in Discord as history; no new external work is requested.
func (s *DiscordIntegrationService) Remove(ctx context.Context, p Principal) error {
	if !p.CanManageIntegrations() {
		return repository.ErrAccessDenied
	}

	var credential string

	err := database.Transaction(s.db.WithContext(ctx), func(tx *gorm.DB) error {
		store := repository.NewIntegrationRepository(tx)

		connection, err := discordConnection(ctx, store)
		if err != nil || connection.ID == "" {
			return err
		}

		credential = connection.CredentialRef

		return store.DeleteConnection(ctx, connection.ID)
	})
	if err != nil {
		return err
	}

	if credential != "" {
		if err := s.secrets.Delete(credential); err != nil {
			slog.Warn("retire removed integration credential", "error", err)
		}
	}

	return nil
}

// Channels refreshes the checked destinations for the saved connection.
func (s *DiscordIntegrationService) Channels(ctx context.Context, p Principal) ([]discord.Channel, error) {
	status, err := s.Status(ctx, p)
	if err != nil {
		return nil, err
	}

	if !status.Configured || !status.Enabled {
		return nil, repository.ErrIntegrationDisabled
	}

	if status.Error != "" && len(status.Channels) == 0 {
		return nil, &discord.Error{Message: status.Error}
	}

	return status.Channels, nil
}

// Test sends one explicit administrator-requested message to a checked channel.
func (s *DiscordIntegrationService) Test(ctx context.Context, p Principal, channelID string) error {
	connection, err := s.connection(ctx, p)
	if err != nil {
		return err
	}

	if connection.ID == "" || !connection.Enabled {
		return repository.ErrIntegrationDisabled
	}

	var config discord.Configuration
	if json.Unmarshal(connection.Config, &config) != nil {
		return ErrIntegrationInput
	}

	token, err := s.token(connection, "")
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	return s.client.TestMessage(ctx, token, config.GuildID, channelID)
}
