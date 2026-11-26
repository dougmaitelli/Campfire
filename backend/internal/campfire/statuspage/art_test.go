package statuspage

import "testing"

func TestPublicArtworkExcludesPrivateEndpointsAndUnsafeURLs(t *testing.T) {
	for _, value := range []string{"/game-art/mock-block.svg", "/assets/game.png", "https://images.example.test/game.webp", "https://images.example.test/"} {
		if got := PublicArtURL(value); got != value {
			t.Fatal("valid artwork rejected", value, got)
		}
	}

	for _, value := range []string{"", "//example.test/a.png", "http://example.test/a.png", "javascript:alert(1)", "https://user:secret@example.test/art.png", "/api/v1/servers/game/files", "https://panel.example.test/api/v1/servers/game/files", "/game-art/../api/v1/private", "/game-art/%2e%2e/api/v1/private", "/api%2fv1/private", "/spark/private", "/downloads/private", "/__test/reset",
		`https://panel.example.test/game-art\..\api/v1/private`,
		`https://panel.example.test/\api/v1/private`,
		`/game-art\..\api/v1/private`,
		"https://panel.example.test/game-art%5c..%5capi/v1/private",
		"/game-art/%5C../api/v1/private",
	} {
		if got := PublicArtURL(value); got != "" {
			t.Fatal("unsafe artwork published", value, got)
		}
	}
}
