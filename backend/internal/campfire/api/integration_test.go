package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"campfire/internal/campfire/api"
	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	fixture "campfire/internal/campfire/devfixture/integrations"
	core "campfire/internal/campfire/integrations"
	"campfire/internal/campfire/service"
	"campfire/internal/campfire/service/integrations"
	"campfire/internal/campfire/service/integrations/discord"
	"campfire/internal/shared/model"
)

func TestDiscordIntegrationAPIUsesAdminBoundaryAndRedactsCredentials(t *testing.T) {
	db, err := database.Open(t.Context(), filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	remote := httptest.NewServer(fixture.DiscordHandler())
	defer remote.Close()

	setup := integrations.NewDiscordIntegrationService(db.ORM(), integrations.Secrets{Directory: filepath.Join(t.TempDir(), "credentials")}, discord.NewClient(remote.URL, remote.Client()))
	panel := api.New(nil, nil, nil).WithIntegrations(setup)
	admin := authenticatedTestAPI(t, db, panel)
	expectAccess(t, admin, "GET", "/api/v1/integrations/discord", "", http.StatusOK)
	expectAccess(t, panel.Handler(), "GET", "/api/v1/integrations/discord", "", http.StatusUnauthorized)
	_, raw := createTestAPIToken(t, admin, `["notifications.manage"]`)
	limited := bearerAPI(panel.Handler(), raw)

	memberBody, err := json.Marshal(map[string]any{"username": "discord-member", "password": testAdminPassword, "role_id": database.UserRole})
	if err != nil {
		t.Fatal(err)
	}

	expectAccess(t, admin, "POST", "/api/v1/users", string(memberBody), http.StatusCreated)

	auth := service.NewAuthService(repository.NewAuthRepository(db.ORM()))

	session, token, err := auth.Login(t.Context(), "discord-member", testAdminPassword, "")
	if err != nil {
		t.Fatal(err)
	}

	member := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.AddCookie(&http.Cookie{Name: "campfire_session", Value: token})
		r.Header.Set("X-CSRF-Token", session.CSRF)
		panel.Handler().ServeHTTP(w, r)
	})

	for _, endpoint := range []struct{ method, path, body string }{
		{"GET", "/api/v1/integrations/discord", ""},
		{"POST", "/api/v1/integrations/discord/preview", `{"token":"private"}`},
		{"PUT", "/api/v1/integrations/discord", `{"enabled":true,"guild_id":"200","token":"private"}`},
		{"DELETE", "/api/v1/integrations/discord", ""},
		{"GET", "/api/v1/integrations/discord/channels", ""},
		{"POST", "/api/v1/integrations/discord/test", `{"channel_id":"400"}`},
	} {
		expectAccess(t, limited, endpoint.method, endpoint.path, endpoint.body, http.StatusForbidden)
		expectAccess(t, member, endpoint.method, endpoint.path, endpoint.body, http.StatusForbidden)
	}

	body, err := json.Marshal(map[string]string{"token": fixture.DiscordToken})
	if err != nil {
		t.Fatal(err)
	}

	preview := request(admin, "POST", "/api/v1/integrations/discord/preview", string(body))
	if preview.Code != http.StatusOK || !strings.Contains(preview.Body.String(), "Campfire Community") || strings.Contains(preview.Body.String(), fixture.DiscordToken) {
		t.Fatal("bot preview failed or exposed its token", preview.Code, preview.Body)
	}

	payload, err := json.Marshal(integrations.DiscordIntegrationInput{Enabled: true, GuildID: "200", Token: fixture.DiscordToken})
	if err != nil {
		t.Fatal(err)
	}

	response := request(admin, "PUT", "/api/v1/integrations/discord", string(payload))
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), fixture.DiscordToken) || strings.Contains(response.Body.String(), "credential") {
		t.Fatal("configuration save failed or exposed credential data", response.Code, response.Body)
	}

	expectAccess(t, admin, "GET", "/api/v1/integrations/discord/channels", "", http.StatusOK)
	expectAccess(t, admin, "POST", "/api/v1/integrations/discord/test", `{"channel_id":"401"}`, http.StatusBadRequest)
	expectAccess(t, admin, "POST", "/api/v1/integrations/discord/test", `{"channel_id":"400"}`, http.StatusNoContent)
	expectAccess(t, admin, "DELETE", "/api/v1/integrations/discord", "", http.StatusNoContent)
	expectAccess(t, admin, "DELETE", "/api/v1/integrations/discord", "", http.StatusNoContent)

	removed := request(admin, "GET", "/api/v1/integrations/discord", "")
	if !strings.Contains(removed.Body.String(), `"configured":false`) {
		t.Fatal("removed integration remains configured", removed.Body.String())
	}
}

func TestServerDiscordIntegrationAPIEnforcesResourceAndSourceGrants(t *testing.T) {
	f := newAccessFixture(t)
	_, _, input := serverFixture(t, f.admin)
	created := request(f.admin, "POST", "/api/v1/servers", configJSON(t, input))

	var server model.Server
	if created.Code != http.StatusCreated || json.Unmarshal(created.Body.Bytes(), &server) != nil {
		t.Fatal(created.Code, created.Body)
	}

	remote := httptest.NewServer(fixture.DiscordHandler())
	defer remote.Close()

	setup := integrations.NewDiscordIntegrationService(f.db.ORM(), integrations.Secrets{Directory: filepath.Join(t.TempDir(), "credentials")}, discord.NewClient(remote.URL, remote.Client()))
	servers := service.NewServerService(repository.NewServerRepository(f.db.ORM()), nil)
	panel := api.New(nil, nil, servers).WithIntegrations(setup).WithDeployment(&service.DeploymentService{Servers: servers})
	admin := authenticatedTestAPI(t, f.db, panel)
	expectAccess(t, admin, "PUT", "/api/v1/integrations/discord", `{"enabled":true,"guild_id":"200","token":"campfire-test-discord-token"}`, http.StatusOK)

	base := "/api/v1/servers/" + server.ID + "/integrations/discord"
	settings := `{"enabled":true,"channel_id":"400","subscriptions":[{"category":"game"},{"category":"power"}],"card":{"enabled":true,"fields":["players","field.invite_code"]}}`
	createdBinding := request(admin, "POST", base, settings)

	var binding integrations.ServerIntegrationSettings
	if createdBinding.Code != http.StatusCreated || json.Unmarshal(createdBinding.Body.Bytes(), &binding) != nil || binding.ID == "" {
		t.Fatal(createdBinding.Code, createdBinding.Body)
	}

	endpoint := base + "/" + binding.ID
	expectAccess(t, admin, "PUT", endpoint, strings.Replace(settings, `"category":"game"`, `"category":"game","types":["player.joined"]`, 1), http.StatusBadRequest)
	_, narrow := createTestAPIToken(t, admin, `["server.view"]`)
	_, manager := createTestAPIToken(t, admin, `["server.view","server.console.read","server.operate","server.configure"]`)
	limited := bearerAPI(panel.Handler(), narrow)
	allowed := bearerAPI(panel.Handler(), manager)

	choices := request(allowed, "GET", base, "")

	var overview integrations.ServerIntegrationList
	if choices.Code != http.StatusOK || json.Unmarshal(choices.Body.Bytes(), &overview) != nil || len(overview.Connections) != 1 || overview.Connections[0].ID != binding.IntegrationID || overview.Connections[0].Name != "Campfire Community" || strings.Contains(choices.Body.String(), "credential") || strings.Contains(choices.Body.String(), fixture.DiscordToken) {
		t.Fatal("invalid or private connection choices", choices.Code, choices.Body)
	}

	expectAccess(t, allowed, "GET", base+"/channels?integration_id="+binding.IntegrationID, "", http.StatusOK)
	expectAccess(t, limited, "GET", base+"/channels?integration_id="+binding.IntegrationID, "", http.StatusForbidden)
	expectAccess(t, allowed, "GET", base+"/channels?integration_id=missing", "", http.StatusNotFound)

	for _, check := range []struct{ method, path, body string }{
		{"GET", base, ""}, {"GET", endpoint, ""}, {"GET", base + "/channels", ""}, {"PUT", endpoint, settings}, {"GET", endpoint + "/messages", ""}, {"GET", base + "/card?field=players", ""},
		{"PUT", base, fmt.Sprintf(`{"upserts":[{"id":%q,"settings":%s}],"remove_ids":["already-removed"]}`, binding.ID, settings)},
	} {
		expectAccess(t, limited, check.method, check.path, check.body, http.StatusForbidden)
		expectAccess(t, allowed, check.method, check.path, check.body, http.StatusOK)
		expectAccess(t, panel.Handler(), check.method, check.path, check.body, http.StatusUnauthorized)
	}

	expectAccess(t, allowed, "PUT", base, `{"upserts":[{"id":"","settings":{}}]}`, http.StatusBadRequest)
	expectAccess(t, allowed, "PUT", base, fmt.Sprintf(`{"upserts":[{"id":%q,"settings":%s}],"remove_ids":[%q]}`, binding.ID, settings, binding.ID), http.StatusBadRequest)

	preview := request(allowed, "GET", base+"/card?field=players&field=field.world", "")

	var card struct {
		core.ServerCard
		Preview        discord.CardMessage `json:"preview"`
		ExpiredPreview discord.CardMessage `json:"expired_preview"`
	}
	if preview.Code != http.StatusOK || json.Unmarshal(preview.Body.Bytes(), &card) != nil || len(card.Fields) != 2 || card.Fields[0].Available || card.Fields[1].Available || strings.Contains(preview.Body.String(), "variables") || strings.Contains(preview.Body.String(), "credential") {
		t.Fatal("invalid or private card preview", preview.Code, preview.Body)
	}

	if len(card.Preview.Embeds) != 1 || len(card.ExpiredPreview.Embeds) != 1 || card.Preview.Embeds[0].Fields[0].Value != "🔴 unknown" || card.Preview.Embeds[0].Fields[1].Name != "Players" || card.ExpiredPreview.Embeds[0].Fields[1].Value != "Unavailable" {
		t.Fatal("missing rendered live or expired presentation", preview.Body)
	}

	expectAccess(t, allowed, "GET", base+"/card?field=variables.token", "", http.StatusBadRequest)
	expectAccess(t, allowed, "GET", base+"/card?field=players&field=players", "", http.StatusBadRequest)

	unchanged := request(allowed, "GET", endpoint, "")
	if strings.Contains(unchanged.Body.String(), "field.world") {
		t.Fatal("preview mutated saved field selection", unchanged.Body)
	}

	expectAccess(t, allowed, "GET", "/api/v1/integrations/discord", "", http.StatusForbidden)
	expectAccess(t, allowed, "PUT", endpoint, strings.Replace(settings, `"game"`, `"backup"`, 1), http.StatusForbidden)
	expectAccess(t, allowed, "PUT", endpoint, strings.Replace(settings, `"game"`, `"console_alert"`, 1), http.StatusOK)
	expectAccess(t, allowed, "PUT", endpoint, settings, http.StatusOK)
	expectAccess(t, allowed, "GET", "/api/v1/servers/nonexistent/integrations/discord", "", http.StatusNotFound)

	store := repository.NewIntegrationRepository(f.db.ORM())

	claim, err := store.ClaimCard(t.Context(), time.Now())
	if err != nil || claim == nil {
		t.Fatal(claim, err)
	}

	work := core.CardWork{Version: 1, CreationID: "private-creation-identity", Creating: true, CreatedAt: time.Now().Unix()}
	if err := store.FinishCard(t.Context(), *claim, work, core.MessageReference{}, model.DeliveryFailed, "Card needs a receipt check", time.Time{}); err != nil {
		t.Fatal(err)
	}

	cardRetry := endpoint + "/card/retry"
	expectAccess(t, limited, "POST", cardRetry, `{}`, http.StatusForbidden)
	expectAccess(t, panel.Handler(), "POST", cardRetry, `{}`, http.StatusUnauthorized)
	expectAccess(t, allowed, "POST", cardRetry, `{}`, http.StatusNoContent)
	expectAccess(t, allowed, "POST", cardRetry, `{}`, http.StatusConflict)

	cardSettings := request(allowed, "GET", endpoint, "")
	if strings.Contains(cardSettings.Body.String(), "private-creation-identity") || !strings.Contains(cardSettings.Body.String(), `"state":"uncertain"`) {
		t.Fatal("card status lost uncertainty or exposed internal identity", cardSettings.Body)
	}

	bindings, err := store.ListBindings(t.Context(), server.ID)
	if err != nil || len(bindings) != 1 {
		t.Fatal(bindings, err)
	}

	publicPayload, err := json.Marshal(core.Event{ID: "game:api", ServerID: server.ID, Category: core.GameEvents, Type: "player.joined", Title: "Public game event", OccurredAt: time.Now().Unix(), Data: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}

	game, err := store.EnqueueMessage(t.Context(), bindings[0].ID, "game:api", core.GameEvents, publicPayload, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	private, err := store.EnqueueMessage(t.Context(), bindings[0].ID, "backup:api", core.BackupEvents, json.RawMessage(`{"title":"Private backup event"}`), time.Now())
	if err != nil {
		t.Fatal(err)
	}

	if err := f.db.ORM().Model(&database.IntegrationMessage{}).Where("id IN ?", []int64{game.ID, private.ID}).Updates(map[string]any{"state": model.DeliveryFailed, "next_attempt_at": 0}).Error; err != nil {
		t.Fatal(err)
	}

	listed := request(allowed, "GET", endpoint+"/messages?state=failed", "")
	if listed.Code != http.StatusOK || strings.Contains(listed.Body.String(), "Private backup") || !strings.Contains(listed.Body.String(), "Public game") {
		t.Fatal("message status source access bypass", listed.Code, listed.Body)
	}

	retry := fmt.Sprintf("%s/messages/%d/retry", endpoint, game.ID)
	expectAccess(t, limited, "POST", retry, `{}`, http.StatusForbidden)
	expectAccess(t, panel.Handler(), "POST", retry, `{}`, http.StatusUnauthorized)
	expectAccess(t, allowed, "POST", fmt.Sprintf("%s/messages/%d/retry", endpoint, private.ID), `{}`, http.StatusNotFound)
	expectAccess(t, allowed, "POST", retry, `{}`, http.StatusNoContent)
	expectAccess(t, allowed, "POST", retry, `{}`, http.StatusConflict)
	expectAccess(t, allowed, "GET", endpoint+"/messages?state=unknown", "", http.StatusBadRequest)
	expectAccess(t, allowed, "GET", endpoint+"/messages?before=-1", "", http.StatusBadRequest)

	user, _ := f.user("integration-manager", database.UserRole)
	response := request(f.admin, "POST", "/api/v1/roles", `{"name":"Server editor","scope":"server","permissions":["server.configure"]}`)

	var role service.RoleView
	if response.Code != http.StatusCreated || json.Unmarshal(response.Body.Bytes(), &role) != nil {
		t.Fatal(response.Code, response.Body)
	}

	expectAccess(t, f.admin, "POST", "/api/v1/servers/"+server.ID+"/access/grants", fmt.Sprintf(`{"username":%q,"access_role_id":%d}`, user.Username, role.ID), http.StatusNoContent)

	session, token, err := f.auth.Login(t.Context(), user.Username, testAdminPassword, "")
	if err != nil {
		t.Fatal(err)
	}

	member := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.AddCookie(&http.Cookie{Name: "campfire_session", Value: token})
		r.Header.Set("X-CSRF-Token", session.CSRF)
		panel.Handler().ServeHTTP(w, r)
	})
	expectAccess(t, member, "GET", base+"/channels", "", http.StatusOK)
	expectAccess(t, member, "PUT", endpoint, settings, http.StatusOK)
	expectAccess(t, member, "GET", "/api/v1/integrations/discord", "", http.StatusForbidden)
	expectAccess(t, f.admin, "DELETE", fmt.Sprintf("/api/v1/servers/%s/members/%d", server.ID, user.ID), "", http.StatusNoContent)
	expectAccess(t, member, "GET", endpoint, "", http.StatusNotFound)
}
