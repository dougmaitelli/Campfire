package discord

import (
	"fmt"
	"reflect"
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

func TestServerCardUsesSharedGameFieldPresentation(t *testing.T) {
	game := &model.GameState{
		Players: new(3), Capacity: new(20),
		Fields:      map[string]any{"world": "Meadows", "raw_key": false, "players": true},
		FieldLabels: model.GameFieldLabels{"world": "World name", "players": "Player tracking"},
	}
	card := core.BuildServerCard(model.Server{}, model.RuntimeStateRunning, game, core.CardSettings{
		Fields: []string{"players", "capacity", "field.world", "field.raw_key", "field.players"},
	}, game.FieldLabels)
	message := RenderServerCard(card)

	want := []embedField{
		{Name: "Status", Value: "🟢 running", Inline: true},
		{Name: "Players", Value: "3 / 20", Inline: true},
		{Name: "World name", Value: "Meadows", Inline: true},
		{Name: `raw\_key`, Value: "No", Inline: true},
		{Name: "Player tracking", Value: "Yes", Inline: true},
	}
	if got := message.Embeds[0].Fields; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestPreviewUsesActualDiscordLimitsAndEmbedSplitting(t *testing.T) {
	card := core.ServerCard{Name: strings.Repeat("Server ", 60), State: model.RuntimeStateRunning}
	for index := range 32 {
		card.Fields = append(card.Fields, core.CardField{ID: fmt.Sprintf("field.value%d", index), Name: strings.Repeat("*", 100), Value: strings.Repeat("value_", 300), Available: true})
	}

	sent := RenderServerCard(card)

	preview := PreviewServerCard(card)
	if len(preview.Embeds) != 2 || len(preview.Embeds) != len(sent.Embeds) {
		t.Fatal("preview lost Discord's continuation embeds", preview)
	}

	for i, embed := range preview.Embeds {
		if embed.Title != sent.Embeds[i].Title || len(embed.Fields) != len(sent.Embeds[i].Fields) {
			t.Fatal("preview has different title limits or field counts", preview)
		}

		for j, field := range embed.Fields {
			actual := sent.Embeds[i].Fields[j]
			if field.Name != strings.ReplaceAll(actual.Name, `\*`, `*`) || field.Value != strings.ReplaceAll(actual.Value, `\_`, `_`) {
				t.Fatal("preview did not project the exact bounded payload", field, actual)
			}
		}
	}

	if card.Fields[0].Name != strings.Repeat("*", 100) || card.Fields[0].Value != strings.Repeat("value_", 300) {
		t.Fatal("rendering mutated the source card")
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
