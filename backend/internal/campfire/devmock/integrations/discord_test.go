//go:build !production

package integrations

import (
	"net/http"
	"net/http/httptest"
	"testing"

	fixture "campfire/internal/campfire/devfixture/integrations"
)

func TestDiscordMockUsesLocalAdapterAndContainsViewerRoutes(t *testing.T) {
	d := NewDiscord()
	defer d.Close()

	client := d.Client()

	access, err := client.Check(t.Context(), fixture.DiscordToken, "200")
	if err != nil || access.Bot.ID != "100" || len(access.Channels) != 3 {
		t.Fatal("local provider is not wired through the real client", access, err)
	}

	if _, err := client.Preview(t.Context(), "a-real-token-would-also-stay-local"); err == nil {
		t.Fatal("mock accepted a non-fixture credential")
	}

	if err := client.TestMessage(t.Context(), fixture.DiscordToken, "200", "400"); err != nil {
		t.Fatal(err)
	}

	handler := d.WrapHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))

	for _, scenario := range []struct {
		path   string
		status int
	}{
		{"/mock/discord", http.StatusOK},
		{"/mock/discord/viewer.js", http.StatusOK},
		{"/mock/discord/state", http.StatusOK},
		{"/mock/discord/unknown", http.StatusNotFound},
		{"/mock/discord-other", http.StatusTeapot},
		{"/api/v1/servers", http.StatusTeapot},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), "GET", scenario.path, nil))

		if response.Code != scenario.status {
			t.Fatal(scenario.path, response.Code)
		}
	}
}
