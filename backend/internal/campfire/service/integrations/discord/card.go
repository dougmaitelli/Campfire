package discord

import (
	"strings"
	"unicode/utf16"

	core "campfire/internal/campfire/integrations"
	"campfire/internal/shared/model"
)

// CardMessage is ready for Discord create/edit requests. Sending and singleton
// reconciliation are separate from rendering; neither occurs during a preview.
type CardMessage struct {
	Embeds          []messageEmbed `json:"embeds"`
	AllowedMentions struct {
		Parse []string `json:"parse"`
	} `json:"allowed_mentions"`
}

// PreviewCard projects the actual Discord payload into plain text for the
// browser. Limits, budgeting, embed splitting, and status rendering all come from
// RenderCard, not a second approximation in the frontend.
func PreviewCard(card core.LiveCard) CardMessage {
	message := RenderCard(card)
	unescape := strings.NewReplacer(
		`\\`, `\`, "\\`", "`", `\*`, `*`, `\_`, `_`, `\~`, `~`,
		`\[`, `[`, `\]`, `]`, `\<`, `<`, `\>`, `>`, `\|`, `|`,
	)

	for i := range message.Embeds {
		embed := &message.Embeds[i]
		embed.Title = unescape.Replace(embed.Title)

		embed.Footer.Text = unescape.Replace(embed.Footer.Text)
		for j := range embed.Fields {
			embed.Fields[j].Name = unescape.Replace(embed.Fields[j].Name)
			embed.Fields[j].Value = unescape.Replace(embed.Fields[j].Value)
		}
	}

	return message
}

// RenderCard preserves every selected field within Discord's field and
// total-text limits. Large selections use a second embed in the same message.
func RenderCard(card core.LiveCard) CardMessage {
	if card.StatusPage {
		return renderStatusPageCard(card)
	}

	state := strings.ReplaceAll(string(card.State), "_", " ")
	if state == "" {
		state = "unknown"
	}

	state = strings.ToUpper(state[:1]) + state[1:]

	fields := []embedField{{Name: "Status", Value: state, Inline: true}}

	addressIndex := -1
	if len(card.Addresses) > 0 {
		addressIndex = len(fields)
		fields = append(fields, embedField{Name: "Address", Value: cardAddressText(card.Addresses)})
	}

	for _, field := range card.Fields {
		value := "Unavailable"
		if field.Available {
			value = field.Value
		}

		fields = append(fields, embedField{Name: field.Name, Value: value, Inline: true})
	}

	color := 0x8b8b8b

	switch card.State {
	case model.RuntimeStateRunning:
		color = 0x80b976
	case model.RuntimeStateDead, model.RuntimeStateUnknown, "":
		color = 0xd46c69
	case model.RuntimeStateStarting, model.RuntimeStateStopping, model.RuntimeStateRestarting, model.RuntimeStateInstalling, model.RuntimeStateUpdating, model.RuntimeStateReinstalling, model.RuntimeStateDeleting, model.RuntimeStateNotInstalled, model.RuntimeStatePaused:
		color = 0xe0ac62
	}

	result := CardMessage{Embeds: []messageEmbed{}}
	result.AllowedMentions.Parse = []string{}

	remaining, readings := 5600, len(fields)
	if addressIndex >= 0 {
		remaining -= len(utf16.Encode([]rune(fields[addressIndex].Name + fields[addressIndex].Value)))
		readings--
	}

	budget := remaining / readings

	for index, field := range fields {
		if index%25 == 0 {
			title := "Additional server details"
			if index == 0 {
				title = card.Name
			}

			embed := messageEmbed{Title: discordText(title, 256), Color: color}
			embed.Footer.Text = "Campfire server"
			result.Embeds = append(result.Embeds, embed)
		}

		// The address is already escaped and budgeted in full. It is a connection
		// string, not a reading that remains useful when shortened.
		if index != addressIndex {
			field.Name = discordText(field.Name, 64)
			field.Value = discordText(field.Value, min(1024, budget-len(utf16.Encode([]rune(field.Name)))))
		}

		result.Embeds[len(result.Embeds)-1].Fields = append(result.Embeds[len(result.Embeds)-1].Fields, field)
	}

	return result
}

// Every selected server fits in one message, including a full 50-entry page.
func renderStatusPageCard(card core.LiveCard) CardMessage {
	result := CardMessage{}
	result.AllowedMentions.Parse = []string{}

	for first := 0; first < max(1, len(card.Fields)); first += 25 {
		title := card.Name
		if first != 0 {
			title = "More servers"
		}

		embed := messageEmbed{Title: discordText(title, 256), Color: 0x5865f2}

		embed.Footer.Text = cardLabel(true)
		if len(card.Fields) == 0 {
			embed.Fields = []embedField{{Name: "Servers", Value: "No servers are listed on this page."}}
		}

		for _, entry := range card.Fields[first:min(first+25, len(card.Fields))] {
			state := "Unknown"

			switch entry.Value {
			case "online":
				state = "Online"
			case "offline":
				state = "Offline"
			}

			embed.Fields = append(embed.Fields, embedField{Name: discordText(entry.Name, min(256, 5500/len(card.Fields)-len(state))), Value: state, Inline: true})
		}

		result.Embeds = append(result.Embeds, embed)
	}

	return result
}

func cardLabel(statusPage bool) string {
	if statusPage {
		return "Campfire status page"
	}

	return "Campfire server"
}

func cardAddressText(addresses []string) string {
	value := escapeDiscordText(strings.Join(addresses, "\n"))
	if len(utf16.Encode([]rune(value))) > 1024 {
		return "Connection address is too long for Discord. Ask the server operator for the complete address."
	}

	return value
}
