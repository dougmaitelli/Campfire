package discord

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf16"

	core "campfire/internal/campfire/integrations"
	"campfire/internal/shared/model"
)

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
