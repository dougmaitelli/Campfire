package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"campfire/internal/campfire/database/repository"
	core "campfire/internal/campfire/integrations"
	"campfire/internal/campfire/service/integrations/discord"
)

type discordProvider struct {
	secrets Secrets
	client  *discord.Client
}

func discordDeliveryFailure(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, ErrSecretUnavailable) {
		return &core.DeliveryFailure{Message: "Integration credentials are unavailable. Check the panel state directory and saved bot connection.", Permanent: true, Blocked: true, Global: true}
	}

	var failure *discord.Failure
	if errors.As(err, &failure) {
		return &core.DeliveryFailure{Message: failure.Error(), RetryAfter: failure.RetryAfter, Global: failure.Global, Permanent: failure.Permanent, Uncertain: failure.Uncertain, Blocked: failure.Blocked}
	}

	var expected *discord.Error
	if errors.As(err, &expected) {
		return &core.DeliveryFailure{Message: expected.Error(), Permanent: true, Blocked: true, Global: true}
	}

	return &core.DeliveryFailure{Message: "The Discord request could not be completed. Delivery will be retried."}
}

func (p *discordProvider) credentials(claim repository.IntegrationClaim) (string, discord.Configuration, error) {
	var config discord.Configuration
	if json.Unmarshal(claim.Connection.Config, &config) != nil || !discord.ValidID(config.BotID) || !discord.ValidID(config.GuildID) {
		return "", config, &core.DeliveryFailure{Message: "Saved Discord settings are invalid. Save the connection again.", Permanent: true, Blocked: true, Global: true}
	}

	token, err := p.secrets.Get(claim.Connection.CredentialRef)

	return token, config, discordDeliveryFailure(err)
}

func (p *discordProvider) Check(ctx context.Context, claim repository.IntegrationClaim) error {
	token, config, err := p.credentials(claim)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	access, err := p.client.CheckDelivery(ctx, token, config.GuildID)
	if err != nil {
		return discordDeliveryFailure(err)
	}

	if access.Bot.ID != config.BotID {
		return &core.DeliveryFailure{Message: "The saved Discord credentials belong to a different bot. Save the connection again.", Permanent: true, Blocked: true, Global: true}
	}

	if !slices.ContainsFunc(access.Channels, func(channel discord.Channel) bool { return channel.ID == claim.DestinationID && channel.Available }) {
		return &core.DeliveryFailure{Message: "The destination no longer has the required Discord permissions.", Permanent: true, Blocked: true}
	}

	return nil
}

func (p *discordProvider) Send(ctx context.Context, claim repository.IntegrationClaim) (string, error) {
	token, _, err := p.credentials(claim)
	if err != nil {
		return "", err
	}

	event, err := readDeliveryEvent(claim)
	if err != nil {
		return "", err
	}

	id, err := p.client.SendEvent(ctx, token, claim.DestinationID, claim.ServerName, discord.MessageNonce(claim.BindingID, claim.EventID), event)

	return id, discordDeliveryFailure(err)
}

func (p *discordProvider) Find(ctx context.Context, claim repository.IntegrationClaim) (string, error) {
	token, config, err := p.credentials(claim)
	if err != nil {
		return "", err
	}

	id, err := p.client.FindEvent(ctx, token, claim.DestinationID, config.BotID, discord.MessageNonce(claim.BindingID, claim.EventID), claim.CreatedAt)

	return id, discordDeliveryFailure(err)
}
