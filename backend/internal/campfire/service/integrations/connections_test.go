package integrations

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"campfire/internal/campfire/database/repository"
	fixture "campfire/internal/campfire/devfixture/integrations"
	accessperm "campfire/internal/shared/permissions"
)

func TestServerConnectionSelectionUsesSavedIdentityAndRequiresAnUnambiguousChoice(t *testing.T) {
	var token atomic.Value

	remote := fixture.DiscordHandler()
	db, secrets, setup := discordService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token.Store(r.Header.Get("Authorization"))
		remote.ServeHTTP(w, r)
	}))
	seedIntegrationServers(t, db)

	ctx, grants := t.Context(), accessperm.All()
	if _, err := setup.Save(ctx, testPrincipal(true), DiscordIntegrationInput{Enabled: true, GuildID: "200", Token: fixture.DiscordToken}); err != nil {
		t.Fatal(err)
	}

	list, err := setup.ListServerIntegrations(ctx, "first", grants)
	if err != nil || len(list.Connections) != 1 || list.Connections[0].Name != "Campfire Community" {
		t.Fatal(list, err)
	}

	input := ServerIntegrationInput{Enabled: true, ChannelID: "400"}

	first, err := setup.SaveServerSettings(ctx, "first", "", grants, input)
	if err != nil || first.IntegrationID != list.Connections[0].ID {
		t.Fatal("sole connection was not selected", first, err)
	}

	store := repository.NewIntegrationRepository(db.ORM())

	second, err := store.GetConnection(ctx, first.IntegrationID)
	if err != nil {
		t.Fatal(err)
	}

	second.ID = "second-discord"

	second.CredentialRef, err = secrets.Put(fixture.DiscordToken + "-rotated")
	if err != nil {
		t.Fatal(err)
	}

	if err := store.CreateConnection(ctx, second); err != nil {
		t.Fatal(err)
	}

	if _, err := setup.SaveServerSettings(ctx, "first", "", grants, input); !errors.Is(err, ErrIntegrationInput) {
		t.Fatal("multiple connections silently selected a default", err)
	}

	if _, err := setup.ServerChannels(ctx, "first", "", grants); !errors.Is(err, ErrIntegrationInput) {
		t.Fatal("ambiguous channel lookup succeeded", err)
	}

	if _, err := setup.ServerChannels(ctx, "first", second.ID, grants); err != nil {
		t.Fatal(err)
	}

	if token.Load() != "Bot "+fixture.DiscordToken+"-rotated" {
		t.Fatal("channel lookup used another connection")
	}

	input.IntegrationID = second.ID

	other, err := setup.SaveServerSettings(ctx, "first", "", grants, input)
	if err != nil || other.IntegrationID != second.ID || other.ID == first.ID {
		t.Fatal("selected connection was not saved", other, err)
	}

	list, err = setup.ListServerIntegrations(ctx, "first", grants)
	if err != nil || len(list.Connections) != 2 || len(list.Items) != 2 {
		t.Fatal(list, err)
	}

	encoded, err := json.Marshal(list)
	if err != nil || strings.Contains(string(encoded), fixture.DiscordToken) || strings.Contains(string(encoded), "credential") || strings.Contains(string(encoded), `"config"`) {
		t.Fatal("connection choices expose private setup", string(encoded), err)
	}

	if _, err := setup.ServerSettings(ctx, "first", other.ID, grants); err != nil {
		t.Fatal("reading a selected binding still requires a singleton", err)
	}

	if _, err := setup.ServerMessages(ctx, "first", other.ID, grants, 0, ""); err != nil {
		t.Fatal("history still requires a singleton", err)
	}

	if _, err := setup.SaveServerSettings(ctx, "first", first.ID, grants, input); !errors.Is(err, ErrIntegrationInput) {
		t.Fatal("editing silently reassigned the connection", err)
	}

	input.IntegrationID = "missing"
	if _, err := setup.SaveServerSettings(ctx, "first", "", grants, input); !errors.Is(err, repository.ErrIntegrationNotFound) {
		t.Fatal("missing connection accepted", err)
	}

	second.Enabled = false
	if err := store.UpdateConnection(ctx, second); err != nil {
		t.Fatal(err)
	}

	input.IntegrationID = second.ID

	input.Card.Enabled = true
	if _, err := setup.SaveServerSettings(ctx, "first", other.ID, grants, input); !errors.Is(err, repository.ErrIntegrationDisabled) {
		t.Fatal("disabled connection accepted new delivery", err)
	}

	input.Enabled = false
	if _, err := setup.SaveServerSettings(ctx, "first", other.ID, grants, input); err != nil {
		t.Fatal("disabled connection cannot disable its binding", err)
	}

	if err := setup.RemoveServerIntegration(ctx, "first", other.ID, grants); err != nil {
		t.Fatal("removal still requires a singleton", err)
	}
}
