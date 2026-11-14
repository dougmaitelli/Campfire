package discord

import (
	"strings"
	"testing"
	"unicode/utf16"

	core "campfire/internal/campfire/integrations"
	"campfire/internal/shared/model"
)

func TestServerCardListRendering(t *testing.T) {
	names := []any{"Alex", "**Sam**", "@everyone", "<@123456>"}
	state := &model.GameState{Fields: map[string]any{"player_names": names, "empty": []any{}, "secret": []any{"not-selected"}}}
	labels := model.GameFieldLabels{"player_names": "Online players", "empty": "Queue"}
	settings := core.CardSettings{Fields: []string{"field.player_names", "field.empty"}}
	card := core.BuildServerCard(model.Server{ServerConfiguration: model.ServerConfiguration{Name: "Game"}}, model.RuntimeStateRunning, state, settings, labels)
	sent := RenderServerCard(card)
	preview := PreviewServerCard(card)

	if len(sent.Embeds[0].Fields) != 3 || len(sent.AllowedMentions.Parse) != 0 || sent.AllowedMentions.Parse == nil {
		t.Fatal("selection or mention protection lost", sent)
	}

	if field := sent.Embeds[0].Fields[1]; field.Name != "Online players" || !strings.Contains(field.Value, `\*\*Sam\*\*`) || !strings.Contains(field.Value, `\<@123456\>`) {
		t.Fatal("list names were not escaped", field)
	}

	if preview.Embeds[0].Fields[1].Value != "Alex, **Sam**, @everyone, <@123456>" || preview.Embeds[0].Fields[2].Value != "None" {
		t.Fatal("preview lost list formatting", preview)
	}

	for _, runtime := range []model.RuntimeState{model.RuntimeStateStopped, model.RuntimeStateRunning} {
		unavailable := core.BuildServerCard(model.Server{}, runtime, nil, settings, labels)
		if RenderServerCard(unavailable).Embeds[0].Fields[1].Value != "Unavailable" {
			t.Fatal("missing list exposed as empty")
		}
	}

	stopped := core.BuildServerCard(model.Server{}, model.RuntimeStateStopped, state, settings, labels)
	if RenderServerCard(stopped).Embeds[0].Fields[1].Value != "Unavailable" {
		t.Fatal("stopped server leaked player names")
	}

	longNames := make([]any, 64)
	for i := range longNames {
		longNames[i] = strings.Repeat("😀*", 20)
	}

	state.Fields["player_names"] = longNames
	card = core.BuildServerCard(model.Server{}, model.RuntimeStateRunning, state, settings, labels)
	sent = RenderServerCard(card)
	preview = PreviewServerCard(card)

	field := sent.Embeds[0].Fields[1]
	if len(utf16.Encode([]rune(field.Value))) > 1024 || !strings.HasSuffix(field.Value, "…") {
		t.Fatal("list was not bounded", field)
	}

	if !strings.HasSuffix(preview.Embeds[0].Fields[1].Value, "…") {
		t.Fatal("preview not truncated like published card")
	}
}
