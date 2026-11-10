package discord

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"campfire/internal/campfire/integrations"
)

type embedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}
type messageEmbed struct {
	Title       string       `json:"title"`
	Description string       `json:"description,omitempty"`
	Timestamp   string       `json:"timestamp,omitempty"`
	Fields      []embedField `json:"fields,omitempty"`
	Color       int          `json:"color,omitempty"`
	Footer      struct {
		Text string `json:"text"`
	} `json:"footer"`
}
type discordMessage struct {
	ID        string    `json:"id"`
	ChannelID string    `json:"channel_id"`
	Timestamp time.Time `json:"timestamp"`
	Author    struct {
		ID string `json:"id"`
	} `json:"author"`
	Embeds []messageEmbed `json:"embeds"`
}

// MessageNonce stays stable across retries and panel restarts (Discord's limit is 25).
func MessageNonce(bindingID, eventID string) string {
	digest := sha256.Sum256([]byte(bindingID + "\x00" + eventID))
	return hex.EncodeToString(digest[:12])
}

func discordText(value string, limit int) string {
	value = strings.NewReplacer("\\", "\\\\", "`", "\\`", "*", "\\*", "_", "\\_", "~", "\\~", "[", "\\[", "]", "\\]", "<", "\\<", ">", "\\>", "|", "\\|").Replace(value)
	units := 0

	var text strings.Builder

	for _, runeValue := range value {
		width := utf16.RuneLen(runeValue)
		if units+width > limit-1 {
			text.WriteRune('…')
			break
		}

		text.WriteRune(runeValue)

		units += width
	}

	return text.String()
}

func notificationEmbed(serverName string, event integrations.Event, nonce string) messageEmbed {
	embed := messageEmbed{Title: discordText(serverName+": "+event.Title, 256), Description: discordText(event.Body, 2000), Timestamp: time.Unix(event.OccurredAt, 0).UTC().Format(time.RFC3339)}
	embed.Footer.Text = "Campfire notification · " + nonce

	keys := make([]string, 0, len(event.Data))
	for key := range event.Data {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	for _, key := range keys[:min(10, len(keys))] {
		value, ok := integrations.ScalarText(event.Data[key])
		if !ok || value == "" {
			continue
		}

		embed.Fields = append(embed.Fields, embedField{Name: discordText(event.FieldLabels.Label(key), 128), Value: discordText(value, 240)})
	}

	return embed
}

// SendEvent uses Discord's nonce deduplication for one immediate retry of an
// ambiguous response. Later recovery searches for the durable footer reference.
func (c *Client) SendEvent(ctx context.Context, token, channelID, serverName, nonce string, event integrations.Event) (string, error) {
	if !ValidID(channelID) || event.Validate() != nil {
		return "", &Error{"Invalid notification destination or payload."}
	}

	payload := struct {
		Embeds          []messageEmbed `json:"embeds"`
		Nonce           string         `json:"nonce"`
		EnforceNonce    bool           `json:"enforce_nonce"`
		AllowedMentions struct {
			Parse []string `json:"parse"`
		} `json:"allowed_mentions"`
	}{Embeds: []messageEmbed{notificationEmbed(serverName, event, nonce)}, Nonce: nonce, EnforceNonce: true}
	payload.AllowedMentions.Parse = []string{}

	return c.sendMessage(ctx, token, channelID, payload)
}

// sendMessage retries ambiguous creates immediately with the same enforced
// nonce. Both notifications and cards use the same conservative send policy.
func (c *Client) sendMessage(ctx context.Context, token, channelID string, payload any) (string, error) {
	var uncertain error

	for range 2 {
		var result discordMessage

		err := c.request(ctx, token, http.MethodPost, "/channels/"+channelID+"/messages", payload, &result)
		if err == nil && ValidID(result.ID) && result.ChannelID == channelID {
			return result.ID, nil
		}

		if err == nil {
			err = &Failure{Cause: &Error{"Discord returned an invalid message receipt."}, Uncertain: true}
		}

		var failure *Failure
		if errors.As(err, &failure) && failure.Uncertain && ctx.Err() == nil {
			uncertain = err
			continue
		}

		if uncertain != nil {
			var original *Failure
			if errors.As(uncertain, &original) && errors.As(err, &failure) {
				return "", &Failure{Cause: original.Cause, Uncertain: true, RetryAfter: failure.RetryAfter, Global: failure.Global}
			}

			return "", uncertain
		}

		return "", err
	}

	return "", uncertain
}

// FindEvent searches bounded channel history. Missing/inaccessible evidence never
// authorizes a replacement POST after the short nonce window has passed.
func (c *Client) FindEvent(ctx context.Context, token, channelID, botID, nonce string, since int64) (string, error) {
	return c.findMessage(ctx, token, channelID, botID, "Campfire notification · "+nonce, since)
}

func (c *Client) findMessage(ctx context.Context, token, channelID, botID, marker string, since int64) (string, error) {
	if !ValidID(channelID) || !ValidID(botID) {
		return "", &Error{"Invalid notification destination."}
	}

	before := ""

	for range 5 {
		path := "/channels/" + channelID + "/messages?limit=100"
		if before != "" {
			path += "&before=" + before
		}

		var messages []discordMessage
		if err := c.request(ctx, token, "GET", path, nil, &messages); err != nil {
			return "", err
		}

		for _, message := range messages {
			if message.ChannelID == channelID && message.Author.ID == botID && ValidID(message.ID) && slices.ContainsFunc(message.Embeds, func(embed messageEmbed) bool { return embed.Footer.Text == marker }) {
				return message.ID, nil
			}
		}

		if len(messages) < 100 {
			return "", nil
		}

		last := messages[len(messages)-1]
		if !last.Timestamp.IsZero() && last.Timestamp.Unix() < since-5 {
			return "", nil
		}

		if !ValidID(last.ID) || last.ID == before {
			break
		}

		if before != "" {
			old, _ := strconv.ParseUint(before, 10, 64)

			next, _ := strconv.ParseUint(last.ID, 10, 64)
			if next >= old {
				break
			}
		}

		before = last.ID
	}

	return "", &Failure{Cause: &Error{"Delivery could not be confirmed in recent channel history. Automatic resending is paused to avoid duplicates."}, Permanent: true, Uncertain: true}
}
