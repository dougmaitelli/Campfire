package discord

import (
	"context"
	"errors"
	"net/http"
	"slices"

	core "campfire/internal/campfire/integrations"
)

func referencedCard(card core.LiveCard, nonce string) CardMessage {
	message := RenderCard(card)
	for index := range message.Embeds {
		message.Embeds[index].Footer.Text = cardLabel(card.StatusPage) + " · " + nonce
	}

	return message
}

func unknownMessage(err error) bool {
	var failure *Failure
	return errors.As(err, &failure) && failure.UnknownMessage
}

// CreateCard uses the persisted creation identity for immediate deduplication.
func (c *Client) CreateCard(ctx context.Context, token, channelID, nonce string, card core.LiveCard) (string, error) {
	payload := struct {
		CardMessage
		Nonce        string `json:"nonce"`
		EnforceNonce bool   `json:"enforce_nonce"`
	}{CardMessage: referencedCard(card, nonce), Nonce: nonce, EnforceNonce: true}

	return c.sendMessage(ctx, token, channelID, payload)
}

// FindCard never creates a replacement for an uncertain POST.
func (c *Client) FindCard(ctx context.Context, token, channelID, botID, nonce string, since int64, statusPage bool) (string, error) {
	return c.findMessage(ctx, token, channelID, botID, cardLabel(statusPage)+" · "+nonce, since)
}

// CardExists treats only Discord's Unknown Message code as deletion.
// Missing channels, permission failures, and network errors remain errors.
func (c *Client) CardExists(ctx context.Context, token, channelID, messageID, botID, nonce string, statusPage bool) (bool, error) {
	var message discordMessage

	err := c.request(ctx, token, http.MethodGet, "/channels/"+channelID+"/messages/"+messageID, nil, &message)
	if unknownMessage(err) {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	if message.ID != messageID || message.ChannelID != channelID || message.Author.ID != botID || (nonce != "" && !slices.ContainsFunc(message.Embeds, func(embed messageEmbed) bool { return embed.Footer.Text == cardLabel(statusPage)+" · "+nonce })) {
		return false, &Failure{Cause: &Error{"The saved card message identity could not be verified. Automatic replacement is paused."}, Permanent: true}
	}

	return true, nil
}

// EditCard replaces the entire embed list and preserves mention protection.
func (c *Client) EditCard(ctx context.Context, token, channelID, messageID, nonce string, card core.LiveCard) (bool, error) {
	err := c.request(ctx, token, http.MethodPatch, "/channels/"+channelID+"/messages/"+messageID, referencedCard(card, nonce), nil)
	if unknownMessage(err) {
		return false, nil
	}

	return err == nil, err
}

// DeleteCard is idempotent; an ambiguous DELETE can be safely repeated.
func (c *Client) DeleteCard(ctx context.Context, token, channelID, messageID string) error {
	err := c.request(ctx, token, http.MethodDelete, "/channels/"+channelID+"/messages/"+messageID, nil, nil)
	if unknownMessage(err) {
		return nil
	}

	return err
}
