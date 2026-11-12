package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"campfire/internal/campfire/api"
	"campfire/internal/campfire/database"
	"campfire/internal/campfire/service/integrations"
	"campfire/internal/campfire/service/integrations/discord"
)

func TestDiscordPreviewPreservesInviteWhenGuildDiscoveryFails(t *testing.T) {
	db, err := database.Open(t.Context(), filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") == "Bot invalid-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/users/@me" {
			_ = json.NewEncoder(w).Encode(discord.User{ID: "100", Username: "new-bot", Bot: true})
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"private provider response"}`))
	}))
	defer remote.Close()
	setup := integrations.NewDiscordIntegrationService(db.ORM(), integrations.Secrets{Directory: t.TempDir()}, discord.NewClient(remote.URL, remote.Client()))
	admin := authenticatedTestAPI(t, db, api.New(nil, nil, nil).WithIntegrations(setup))
	response := request(admin, "POST", "/api/v1/integrations/discord/preview", `{"token":"private-token"}`)
	var preview discord.Preview
	if err := json.Unmarshal(response.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || preview.InviteURL != discord.BotInviteURL("100", "") || preview.GuildError == "" {
		t.Fatal("invite lost after guild lookup failure", response.Code, response.Body)
	}
	if strings.Contains(response.Body.String(), "private") {
		t.Fatal("preview exposed private data", response.Body)
	}
	response = request(admin, "POST", "/api/v1/integrations/discord/preview", `{"token":"invalid-token"}`)
	if response.Code == http.StatusOK || strings.Contains(response.Body.String(), "invite_url") {
		t.Fatal("invalid bot token produced an invite", response.Code, response.Body)
	}
}
