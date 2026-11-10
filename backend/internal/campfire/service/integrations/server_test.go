package integrations

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	fixture "campfire/internal/campfire/devfixture/integrations"
	integrationmodel "campfire/internal/campfire/integrations"
	"campfire/internal/shared/model"
	accessperm "campfire/internal/shared/permissions"
)

func seedIntegrationServers(t *testing.T, db *database.Database) {
	t.Helper()

	for _, row := range []any{
		&database.Spark{ID: "node", Name: "Node"},
		&database.Ember{ID: "game", Name: "Game", Definition: model.Ember{Schema: 1}},
		&database.Server{ID: "first", Name: "First", SparkID: "node", EmberID: "game", Variables: map[string]string{}},
		&database.Server{ID: "second", Name: "Second", SparkID: "node", EmberID: "game", Variables: map[string]string{}},
	} {
		if err := db.ORM().Omit("Spark", "Ember", "Owner").Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func TestRemovingConnectionCascadesBindingsAndRetiresCredentialsOffline(t *testing.T) {
	var offline atomic.Bool

	remote := fixture.DiscordHandler()
	db, secrets, setup := discordService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if offline.Load() {
			t.Error("removal contacted Discord")
			w.WriteHeader(http.StatusServiceUnavailable)

			return
		}

		remote.ServeHTTP(w, r)
	}))
	seedIntegrationServers(t, db)

	ctx := t.Context()
	if _, err := setup.Save(ctx, testPrincipal(true), DiscordIntegrationInput{Enabled: true, GuildID: "200", Token: fixture.DiscordToken}); err != nil {
		t.Fatal(err)
	}

	if _, err := setup.saveSingleServerSettings(ctx, "first", accessperm.All(), ServerIntegrationInput{Enabled: true, ChannelID: "400", Card: integrationmodel.CardSettings{Enabled: true}}); err != nil {
		t.Fatal(err)
	}

	store := repository.NewIntegrationRepository(db.ORM())

	connections, err := store.ListConnections(ctx)
	if err != nil || len(connections) != 1 {
		t.Fatal(connections, err)
	}

	bindings, err := store.ListBindings(ctx, "first")
	if err != nil || len(bindings) != 1 {
		t.Fatal(bindings, err)
	}

	if _, err := store.EnqueueMessage(ctx, bindings[0].ID, "game:remove", integrationmodel.GameEvents, json.RawMessage(`{"title":"Pending"}`), time.Now()); err != nil {
		t.Fatal(err)
	}

	if err := setup.Remove(ctx, testPrincipal(false)); !errors.Is(err, repository.ErrAccessDenied) {
		t.Fatal("removal bypassed authorization", err)
	}

	offline.Store(true)

	if err := setup.Remove(ctx, testPrincipal(true)); err != nil {
		t.Fatal(err)
	}

	if err := setup.Remove(ctx, testPrincipal(true)); err != nil {
		t.Fatal("removal is not idempotent", err)
	}

	for _, row := range []any{&database.Integration{}, &database.ServerIntegration{}, &database.IntegrationMessage{}} {
		var count int64
		if err := db.ORM().Model(row).Count(&count).Error; err != nil || count != 0 {
			t.Fatal("removal left saved integration data", count, err)
		}
	}

	if _, err := secrets.Get(connections[0].CredentialRef); !errors.Is(err, ErrSecretUnavailable) {
		t.Fatal("removed token remains available", err)
	}

	var servers int64
	if err := db.ORM().Model(&database.Server{}).Where("id = ?", "first").Count(&servers).Error; err != nil || servers != 1 {
		t.Fatal("removal changed the game server", servers, err)
	}
}

func TestServerIntegrationSettingsPreserveIdentityAndCanDisableOffline(t *testing.T) {
	var offline atomic.Bool

	remote := fixture.DiscordHandler()
	db, _, setup := discordService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if offline.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		remote.ServeHTTP(w, r)
	}))
	seedIntegrationServers(t, db)
	ctx := t.Context()

	grants := accessperm.All()
	if status, err := setup.singleServerSettings(ctx, "first", grants); err != nil || status.Configured || status.Enabled {
		t.Fatal("unconfigured settings", status, err)
	}

	if _, err := setup.Save(ctx, testPrincipal(true), DiscordIntegrationInput{Enabled: true, GuildID: "200", Token: fixture.DiscordToken}); err != nil {
		t.Fatal(err)
	}

	input := ServerIntegrationInput{Enabled: true, ChannelID: "400", Subscriptions: []integrationmodel.Subscription{{Category: integrationmodel.GameEvents, Rules: []integrationmodel.EventRule{{Type: "player.joined", Fields: map[string]string{"state": "ready"}}}}}, Card: integrationmodel.CardSettings{Enabled: true, Fields: []string{"players", "field.invite_code"}}}
	for _, id := range []string{"first", "second"} {
		if _, err := setup.saveSingleServerSettings(ctx, id, grants, input); err != nil {
			t.Fatal("shared channel rejected", err)
		}
	}

	store := repository.NewIntegrationRepository(db.ORM())

	bindings, err := store.ListBindings(ctx, "first")
	if err != nil || len(bindings) != 1 {
		t.Fatal(bindings, err)
	}

	ref := integrationmodel.MessageReference{DestinationID: "400", ID: "500"}
	if err := seedIntegrationCardReference(ctx, db, bindings[0].ID, ref); err != nil {
		t.Fatal(err)
	}

	for _, invalid := range []string{"401", "999"} {
		input.ChannelID = invalid
		if _, err := setup.saveSingleServerSettings(ctx, "first", grants, input); err == nil {
			t.Fatal("unavailable or external channel accepted", invalid)
		}
	}

	offline.Store(true)

	input.Enabled, input.ChannelID = false, "401"
	if _, err := setup.saveSingleServerSettings(ctx, "first", grants, input); err != nil {
		t.Fatal("offline disable failed", err)
	}

	settings, err := setup.singleServerSettings(ctx, "first", grants)
	if err != nil || settings.Enabled || settings.ChannelID != "401" || !settings.Card.Enabled || settings.Card.Fields[1] != "field.invite_code" || settings.Subscriptions[0].Rules[0].Fields["state"] != "ready" {
		t.Fatal("settings lost", settings, err)
	}

	saved, err := store.GetBinding(ctx, bindings[0].ID)
	if err != nil || saved.StatusMessage != ref {
		t.Fatal("channel change discarded card cleanup identity", saved, err)
	}

	other, err := setup.singleServerSettings(ctx, "second", grants)
	if err != nil || !other.Enabled || other.ChannelID != "400" {
		t.Fatal("settings crossed servers", other, err)
	}
}

func TestServerIntegrationRequiresConfigurationAndSourcePermissionsBeforeNetwork(t *testing.T) {
	var calls atomic.Int64

	remote := fixture.DiscordHandler()
	db, _, setup := discordService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); remote.ServeHTTP(w, r) }))
	seedIntegrationServers(t, db)

	ctx := t.Context()
	if _, err := setup.Save(ctx, testPrincipal(true), DiscordIntegrationInput{Enabled: true, GuildID: "200", Token: fixture.DiscordToken}); err != nil {
		t.Fatal(err)
	}

	minimal := []model.PermissionID{accessperm.ServerView, accessperm.ServerConfigure}
	input := ServerIntegrationInput{Enabled: true, ChannelID: "400"}

	calls.Store(0)

	for _, grants := range [][]model.PermissionID{nil, {accessperm.ServerView}, {accessperm.ServerConfigure}, {accessperm.ServerView, accessperm.ServerOperate}} {
		if _, err := setup.singleServerSettings(ctx, "first", grants); !errors.Is(err, repository.ErrAccessDenied) {
			t.Fatal("settings permission bypass", err)
		}

		if _, err := setup.ServerChannels(ctx, "first", "", grants); !errors.Is(err, repository.ErrAccessDenied) {
			t.Fatal("channel permission bypass", err)
		}

		if _, err := setup.saveSingleServerSettings(ctx, "first", grants, input); !errors.Is(err, repository.ErrAccessDenied) {
			t.Fatal("save permission bypass", err)
		}
	}

	for _, category := range []integrationmodel.Category{integrationmodel.ConsoleAlerts, integrationmodel.BackupEvents} {
		input.Subscriptions = []integrationmodel.Subscription{{Category: category}}
		if _, err := setup.saveSingleServerSettings(ctx, "first", minimal, input); !errors.Is(err, repository.ErrAccessDenied) {
			t.Fatal("source permission bypass", category, err)
		}
	}

	if calls.Load() != 0 {
		t.Fatal("unauthorized request reached Discord")
	}

	input.Subscriptions = []integrationmodel.Subscription{{Category: integrationmodel.PowerEvents}, {Category: integrationmodel.GameEvents}}
	if _, err := setup.saveSingleServerSettings(ctx, "first", minimal, input); err != nil {
		t.Fatal("server editor could not configure integration", err)
	}

	input.Card.Fields = []string{"unrecognized"}
	if _, err := setup.saveSingleServerSettings(ctx, "first", minimal, input); !errors.Is(err, ErrIntegrationInput) {
		t.Fatal("unrecognized card field accepted", err)
	}

	input.Card.Fields = nil

	if _, err := setup.Save(ctx, testPrincipal(true), DiscordIntegrationInput{Enabled: false, GuildID: "200"}); err != nil {
		t.Fatal(err)
	}

	input.Enabled = false
	if _, err := setup.saveSingleServerSettings(ctx, "first", minimal, input); err != nil {
		t.Fatal(err)
	}

	input.Enabled = true
	if _, err := setup.saveSingleServerSettings(ctx, "first", minimal, input); !errors.Is(err, repository.ErrIntegrationDisabled) {
		t.Fatal("disabled global connection allowed enable", err)
	}

	input.Enabled = false
	if _, err := setup.saveSingleServerSettings(ctx, "first", minimal, input); err != nil {
		t.Fatal("disabled global connection prevented local disable", err)
	}
}
