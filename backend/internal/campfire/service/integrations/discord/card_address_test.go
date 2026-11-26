package discord

import (
	"strings"
	"testing"
	"unicode/utf16"

	core "campfire/internal/campfire/integrations"
	"campfire/internal/shared/model"
)

func TestCardAddressNeverTruncatesAConnectionString(t *testing.T) {
	for _, address := range []string{
		"https://play.example.test/join?token=" + strings.Repeat("a", 300),
		strings.Repeat("a", 1024), strings.Repeat("😀", 512), strings.Repeat("_", 512),
	} {
		card := core.LiveCard{Name: strings.Repeat("Game", 100), State: model.RuntimeStateRunning, Addresses: []string{address}}
		for range 32 {
			card.Fields = append(card.Fields, core.CardField{Name: strings.Repeat("*", 100), Value: strings.Repeat("😀", 2000), Available: true})
		}

		sent := RenderCard(card)

		preview := PreviewCard(card)
		if preview.Embeds[0].Fields[1].Value != address {
			t.Fatal("connection string was changed", preview.Embeds[0].Fields[1])
		}

		total := 0
		fields := 0

		for _, embed := range sent.Embeds {
			total += len(utf16.Encode([]rune(embed.Title + embed.Footer.Text)))
			if len(embed.Fields) > 25 {
				t.Fatal("too many fields")
			}

			for _, field := range embed.Fields {
				if len(utf16.Encode([]rune(field.Value))) > 1024 {
					t.Fatal("field overflow")
				}

				total += len(utf16.Encode([]rune(field.Name + field.Value)))
				fields++
			}
		}

		if total > 6000 || fields != 34 {
			t.Fatal("lost fields or exceeded message budget", total, fields)
		}

		if len(sent.AllowedMentions.Parse) != 0 || sent.AllowedMentions.Parse == nil {
			t.Fatal("mention protection missing")
		}
	}
}

func TestCardAddressTooLongIsExplainedInsteadOfShortened(t *testing.T) {
	for _, address := range []string{strings.Repeat("a", 1025), strings.Repeat("😀", 513), strings.Repeat("_", 513), "https://play.example.test/" + strings.Repeat("a", 1500)} {
		card := core.LiveCard{Name: "Game", Addresses: []string{address}}
		sent := RenderCard(card)
		preview := PreviewCard(card)

		value := preview.Embeds[0].Fields[1].Value
		if value != "Connection address is too long for Discord. Ask the server operator for the complete address." || value != sent.Embeds[0].Fields[1].Value {
			t.Fatal("missing consistent limitation notice", value)
		}

		if card.Addresses[0] != address {
			t.Fatal("rendering mutated the source address")
		}
	}
}
