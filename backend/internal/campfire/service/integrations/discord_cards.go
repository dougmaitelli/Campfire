package integrations

import (
	"context"

	"campfire/internal/campfire/database/repository"
	core "campfire/internal/campfire/integrations"
	"campfire/internal/campfire/service/integrations/discord"
)

func cardNonce(claim repository.CardClaim) string {
	if claim.Work.CreationID == "" {
		return ""
	}

	return discord.MessageNonce(claim.BindingID, claim.Work.CreationID)
}

func (*discordProvider) Fingerprint(card core.LiveCard) string {
	return cardHash(discord.RenderCard(card))
}

func (p *discordProvider) CreateCard(ctx context.Context, claim repository.CardClaim, card core.LiveCard) (string, error) {
	token, _, err := p.credentials(claim.IntegrationClaim)
	if err != nil {
		return "", err
	}

	id, err := p.client.CreateCard(ctx, token, claim.DestinationID, cardNonce(claim), card)

	return id, discordDeliveryFailure(err)
}

func (p *discordProvider) FindCard(ctx context.Context, claim repository.CardClaim) (string, error) {
	token, config, err := p.credentials(claim.IntegrationClaim)
	if err != nil {
		return "", err
	}

	id, err := p.client.FindCard(ctx, token, claim.DestinationID, config.BotID, cardNonce(claim), claim.Work.CreatedAt, claim.Binding.StatusPageID != "")

	return id, discordDeliveryFailure(err)
}

func (p *discordProvider) CardExists(ctx context.Context, claim repository.CardClaim) (bool, error) {
	token, config, err := p.credentials(claim.IntegrationClaim)
	if err != nil {
		return false, err
	}

	found, err := p.client.CardExists(ctx, token, claim.DestinationID, claim.Binding.StatusMessage.ID, config.BotID, cardNonce(claim), claim.Binding.StatusPageID != "")

	return found, discordDeliveryFailure(err)
}

func (p *discordProvider) EditCard(ctx context.Context, claim repository.CardClaim, card core.LiveCard) (bool, error) {
	token, _, err := p.credentials(claim.IntegrationClaim)
	if err != nil {
		return false, err
	}

	found, err := p.client.EditCard(ctx, token, claim.DestinationID, claim.Binding.StatusMessage.ID, cardNonce(claim), card)

	return found, discordDeliveryFailure(err)
}

func (p *discordProvider) DeleteCard(ctx context.Context, claim repository.CardClaim) error {
	token, _, err := p.credentials(claim.IntegrationClaim)
	if err != nil {
		return err
	}

	return discordDeliveryFailure(p.client.DeleteCard(ctx, token, claim.DestinationID, claim.Binding.StatusMessage.ID))
}
