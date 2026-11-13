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

// PreviewServerCard projects the actual Discord payload into plain text for the
// browser. Limits, budgeting, embed splitting, and status rendering all come from
// RenderServerCard, not a second approximation in the frontend.
func PreviewServerCard(card core.ServerCard) CardMessage {
	message := RenderServerCard(card)
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

// RenderServerCard preserves every selected field within Discord's field and
// total-text limits. Large selections use a second embed in the same message.
func RenderServerCard(card core.ServerCard) CardMessage {
	state := strings.ReplaceAll(string(card.State), "_", " ")
	if state == "" {
		state = "unknown"
	}

	state = strings.ToUpper(state[:1]) + state[1:]

	fields := []embedField{{Name: "Status", Value: state, Inline: true}}
	if len(card.Addresses) > 0 {
		fields = append(fields, embedField{Name: "Address", Value: strings.Join(card.Addresses, "\n")})
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

	budget := 5600 / len(fields)
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

		field.Name = discordText(field.Name, 64)
		field.Value = discordText(field.Value, min(1024, budget-len(utf16.Encode([]rune(field.Name)))))
		result.Embeds[len(result.Embeds)-1].Fields = append(result.Embeds[len(result.Embeds)-1].Fields, field)
	}

	return result
}
