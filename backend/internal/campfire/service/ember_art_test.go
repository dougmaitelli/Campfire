package service

import (
	"encoding/json"
	"strings"
	"testing"

	"campfire/internal/shared/model"
)

func TestEmberArtworkValidationAndNativeImport(t *testing.T) {
	for _, value := range []string{"", "https://cdn.example.test/games/art.webp", "/game-art/game.svg"} {
		if err := model.ValidateArtURL(value); err != nil {
			t.Fatal("valid artwork rejected", value, err)
		}
	}

	for _, value := range []string{"http://example.test/art.png", "javascript:alert(1)", "//example.test/art.png", "https://user:password@example.test/art.png", "/\\example.test/art.png", "https://example.test/a\n.png", strings.Repeat("/", 2049)} {
		if err := model.ValidateArtURL(value); err == nil {
			t.Fatal("invalid artwork accepted", value)
		}
	}

	var definition map[string]any
	if err := json.Unmarshal(eggFixture(t, "native.json"), &definition); err != nil {
		t.Fatal(err)
	}

	definition["artUrl"] = "https://cdn.example.test/game.png"

	data, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}

	preview := PreviewEmber(data, ImportOptions{})
	if len(preview.Errors) != 0 || preview.Ember == nil || preview.Ember.ArtURL != "https://cdn.example.test/game.png" {
		t.Fatal("native import lost or failed to normalize artwork", preview)
	}

	var exported model.Ember
	if err := json.Unmarshal([]byte(preview.Definition), &exported); err != nil || exported.ArtURL != preview.Ember.ArtURL {
		t.Fatal("exported definition lost artwork", exported.ArtURL, err)
	}

	exported.ArtURL = " https://cdn.example.test/game.png "
	if err := validateEmber(&exported); err != nil || exported.ArtURL != preview.Ember.ArtURL {
		t.Fatal("saved artwork URL was not normalized", exported.ArtURL, err)
	}
}
