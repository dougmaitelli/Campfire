package discord

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf16"

	core "campfire/internal/campfire/integrations"
	"campfire/internal/shared/model"
)

func TestServerCardStatusIndicators(t *testing.T) {
	for _, test := range []struct {
		state model.RuntimeState
		value string
		color int
	}{
		{model.RuntimeStateRunning, "🟢 running", 0x80b976},
		{model.RuntimeStateStopped, "⚪ stopped", 0x8b8b8b},
		{model.RuntimeStateExited, "⚪ exited", 0x8b8b8b},
		{model.RuntimeStateDead, "🔴 dead", 0xd46c69},
		{model.RuntimeStateUnknown, "🔴 unknown", 0xd46c69},
		{"", "🔴 unknown", 0xd46c69},
		{model.RuntimeStateNotInstalled, "🟠 not installed", 0xe0ac62},
		{model.RuntimeStatePaused, "🟠 paused", 0xe0ac62},
		{model.RuntimeStateStarting, "🟠 starting", 0xe0ac62},
		{model.RuntimeStateStopping, "🟠 stopping", 0xe0ac62},
		{model.RuntimeStateRestarting, "🟠 restarting", 0xe0ac62},
		{model.RuntimeStateInstalling, "🟠 installing", 0xe0ac62},
		{model.RuntimeStateUpdating, "🟠 updating", 0xe0ac62},
		{model.RuntimeStateReinstalling, "🟠 reinstalling", 0xe0ac62},
		{model.RuntimeStateDeleting, "🟠 deleting", 0xe0ac62},
	} {
		t.Run(string(test.state), func(t *testing.T) {
			message := RenderServerCard(core.ServerCard{Name: "Server", State: test.state})

			status := message.Embeds[0].Fields[0]
			if status.Name != "Status" || status.Value != test.value || message.Embeds[0].Color != test.color {
				t.Fatal("status indicator and embed color are inconsistent", message)
			}
		})
	}
}

func TestServerCardRenderingIsBoundedAndContainsEverySelectedField(t *testing.T) {
	card := core.ServerCard{Name: strings.Repeat("😀 **@everyone**", 100), State: model.RuntimeStateRunning, Addresses: []string{strings.Repeat("games.example.test:25565 / TCP\n", 100)}}
	for index := range 32 {
		card.Fields = append(card.Fields, core.CardField{ID: fmt.Sprintf("field.value%d", index), Name: fmt.Sprintf("field_%d", index), Value: strings.Repeat("😀 **@here** ", 500), Available: index != 31})
	}

	message := RenderServerCard(card)
	if len(message.Embeds) != 2 || message.AllowedMentions.Parse == nil || len(message.AllowedMentions.Parse) != 0 {
		t.Fatal("missing field continuation or mention protection", message)
	}

	units := func(value string) int { return len(utf16.Encode([]rune(value))) }
	total, fields := 0, 0

	for _, embed := range message.Embeds {
		if units(embed.Title) > 256 || len(embed.Fields) > 25 || embed.Color == 0 || embed.Timestamp != "" {
			t.Fatal("invalid or unstable card embed", embed)
		}

		total += units(embed.Title) + units(embed.Footer.Text)
		for _, field := range embed.Fields {
			if units(field.Name) > 256 || units(field.Value) > 1024 || field.Value == "" {
				t.Fatal("invalid field limits", field)
			}

			total += units(field.Name) + units(field.Value)
			fields++
		}
	}

	if total > 6000 || fields != 34 || message.Embeds[1].Fields[len(message.Embeds[1].Fields)-1].Value != "Unavailable" {
		t.Fatal("lost selected fields or exceeded total Discord limit", total, fields)
	}

	if !strings.Contains(message.Embeds[0].Title, `\*\*`) {
		t.Fatal("user markdown was not escaped")
	}
}
