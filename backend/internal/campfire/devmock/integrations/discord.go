//go:build !production

// Package integrations wires local external-service simulators into mock-dev.
package integrations

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"

	fixture "campfire/internal/campfire/devfixture/integrations"
	"campfire/internal/campfire/service/integrations/discord"
)

// Discord owns the disposable provider and its local inspection routes.
type Discord struct {
	fixture *fixture.Discord
	server  *httptest.Server
}

// NewDiscord starts a private loopback API. No request can reach real Discord.
func NewDiscord() *Discord {
	provider := fixture.NewDiscord()
	return &Discord{fixture: provider, server: httptest.NewServer(provider.Handler())}
}

// Client exercises the real adapter against the local provider.
func (d *Discord) Client() *discord.Client {
	return discord.NewClient(d.server.URL, d.server.Client())
}

// Close stops the private listener after the panel's integration workers stop.
func (d *Discord) Close() { d.server.Close() }

// WrapHandler mounts the viewer only in the mock executable.
func (d *Discord) WrapHandler(next http.Handler) http.Handler {
	viewer := d.fixture.ViewerHandler()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/mock/discord" || strings.HasPrefix(r.URL.Path, "/mock/discord/") {
			viewer.ServeHTTP(w, r)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// LogSetup prints the viewer and its deliberately fake setup credential.
func (*Discord) LogSetup() {
	slog.Info("Mock Discord viewer: http://127.0.0.1:8080/mock/discord")
	slog.Info("Mock Discord bot token: " + fixture.DiscordToken)
}
