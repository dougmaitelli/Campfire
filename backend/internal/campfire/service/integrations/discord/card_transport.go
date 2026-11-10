package discord

import (
	"context"
	"errors"
	"net/http"
	"slices"

	core "campfire/internal/campfire/integrations"
)

func referencedCard(card core.ServerCard, nonce string) CardMessage {
	message := RenderServerCard(card)
	for index := range message.Embeds {
		message.Embeds[index].Footer.Text = "Campfire server · " + nonce
	}

	return message
}

func unknownMessage(err error) bool {
	var failure *Failure
	return errors.As(err, &failure) && failure.UnknownMessage
}

// CreateServerCard uses the persisted creation identity for immediate deduplication.
func (c *Client) CreateServerCard(ctx context.Context, token, channelID, nonce string, card core.ServerCard) (string, error) {
	payload := struct {
		CardMessage
		Nonce        string `json:"nonce"`
		EnforceNonce bool   `json:"enforce_nonce"`
	}{CardMessage: referencedCard(card, nonce), Nonce: nonce, EnforceNonce: true}

	return c.sendMessage(ctx, token, channelID, payload)
}

// FindServerCard never creates a replacement for an uncertain POST.
func (c *Client) FindServerCard(ctx context.Context, token, channelID, botID, nonce string, since int64) (string, error) {
	return c.findMessage(ctx, token, channelID, botID, "Campfire server · "+nonce, since)
}

// ServerCardExists treats only Discord's Unknown Message code as deletion.
// Missing channels, permission failures, and network errors remain errors.
func (c *Client) ServerCardExists(ctx context.Context, token, channelID, messageID, botID, nonce string) (bool, error) {
	var message discordMessage

	err := c.request(ctx, token, http.MethodGet, "/channels/"+channelID+"/messages/"+messageID, nil, &message)
	if unknownMessage(err) {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	if message.ID != messageID || message.ChannelID != channelID || message.Author.ID != botID || (nonce != "" && !slices.ContainsFunc(message.Embeds, func(embed messageEmbed) bool { return embed.Footer.Text == "Campfire server · "+nonce })) {
		return false, &Failure{Cause: &Error{"The saved server-card message identity could not be verified. Automatic replacement is paused."}, Permanent: true}
	}

	return true, nil
}

// EditServerCard replaces the entire embed list and preserves mention protection.
func (c *Client) EditServerCard(ctx context.Context, token, channelID, messageID, nonce string, card core.ServerCard) (bool, error) {
	err := c.request(ctx, token, http.MethodPatch, "/channels/"+channelID+"/messages/"+messageID, referencedCard(card, nonce), nil)
	if unknownMessage(err) {
		return false, nil
	}

	return err == nil, err
}

// DeleteServerCard is idempotent; an ambiguous DELETE can be safely repeated.
func (c *Client) DeleteServerCard(ctx context.Context, token, channelID, messageID string) error {
	err := c.request(ctx, token, http.MethodDelete, "/channels/"+channelID+"/messages/"+messageID, nil, nil)
	if unknownMessage(err) {
		return nil
	}

	return err
}
