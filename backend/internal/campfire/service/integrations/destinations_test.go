package integrations

import (
	"errors"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	fixture "campfire/internal/campfire/devfixture/integrations"
	core "campfire/internal/campfire/integrations"
	accessperm "campfire/internal/shared/permissions"
)

func TestIndependentServerDestinationsScopeRoutingHistoryAndRemoval(t *testing.T) {
	db, secrets, setup := discordService(t, fixture.DiscordHandler())
	seedIntegrationServers(t, db)

	ctx, grants := t.Context(), accessperm.All()
	if _, err := setup.Save(ctx, testPrincipal(true), DiscordIntegrationInput{Enabled: true, GuildID: "200", Token: fixture.DiscordToken}); err != nil {
		t.Fatal(err)
	}

	public, err := setup.SaveServerSettings(ctx, "first", "", grants, ServerIntegrationInput{Enabled: true, ChannelID: "400", Subscriptions: []core.Subscription{{Category: core.GameEvents, Rules: []core.EventRule{{Type: "game.announcement"}, {Type: "player.joined", Fields: map[string]string{"player": "doug"}}}}}, Card: core.CardSettings{Enabled: true}})
	if err != nil {
		t.Fatal(err)
	}

	admin, err := setup.SaveServerSettings(ctx, "first", "", grants, ServerIntegrationInput{Enabled: true, ChannelID: "402", Subscriptions: []core.Subscription{{Category: core.BackupEvents}}})
	if err != nil || admin.ID == public.ID {
		t.Fatal(admin, err)
	}

	if _, err := setup.SaveServerSettings(ctx, "first", "", grants, public.ServerIntegrationInput); !errors.Is(err, repository.ErrIntegrationConflict) {
		t.Fatal("duplicate channel accepted", err)
	}

	store := repository.NewIntegrationRepository(db.ORM())
	for _, event := range []core.Event{
		{ID: "game:announcement", ServerID: "first", Category: core.GameEvents, Type: "game.announcement", Title: "Announcement", OccurredAt: time.Now().Unix()},
		{ID: "game:doug", ServerID: "first", Category: core.GameEvents, Type: "player.joined", Title: "Doug joined", Data: map[string]any{"player": "Doug"}, OccurredAt: time.Now().Unix()},
		{ID: "game:alex", ServerID: "first", Category: core.GameEvents, Type: "player.joined", Title: "Alex joined", Data: map[string]any{"player": "Alex"}, OccurredAt: time.Now().Unix()},
		{ID: "operation:backup", ServerID: "first", Category: core.BackupEvents, Type: "backup.succeeded", Title: "Backup complete", OccurredAt: time.Now().Unix()},
	} {
		if err := store.RouteEvent(ctx, event, time.Now()); err != nil {
			t.Fatal(err)
		}
	}

	publicMessages, err := setup.ServerMessages(ctx, "first", public.ID, grants, 0, "")
	if err != nil || len(publicMessages) != 2 {
		t.Fatal("public rules or destination isolation failed", publicMessages, err)
	}

	adminMessages, err := setup.ServerMessages(ctx, "first", admin.ID, grants, 0, "")
	if err != nil || len(adminMessages) != 1 || adminMessages[0].Title != "Backup complete" {
		t.Fatal("admin history crossed destinations", adminMessages, err)
	}

	if _, err := setup.ServerSettings(ctx, "second", public.ID, grants); !errors.Is(err, repository.ErrIntegrationNotFound) {
		t.Fatal("settings crossed servers", err)
	}

	if _, err := setup.SaveServerSettings(ctx, "second", public.ID, grants, public.ServerIntegrationInput); !errors.Is(err, repository.ErrIntegrationNotFound) {
		t.Fatal("edit crossed servers", err)
	}

	if err := setup.RemoveServerIntegration(ctx, "second", public.ID, grants); !errors.Is(err, repository.ErrIntegrationNotFound) {
		t.Fatal("removal crossed servers", err)
	}

	if err := setup.RemoveServerIntegration(ctx, "first", admin.ID, grants); err != nil {
		t.Fatal(err)
	}

	list, err := setup.ListServerIntegrations(ctx, "first", grants)
	if err != nil || len(list.Items) != 1 || list.Items[0].ID != public.ID || !list.Items[0].Card.Enabled {
		t.Fatal("removal affected another destination", list, err)
	}

	var rows []database.IntegrationMessage
	if err := db.ORM().Where("binding_id = ?", admin.ID).Find(&rows).Error; err != nil || len(rows) != 0 {
		t.Fatal("removed destination retained queued work", err)
	}

	connections, err := store.ListConnections(ctx)
	if err != nil || len(connections) != 1 {
		t.Fatal(connections, err)
	}

	if _, err := secrets.Get(connections[0].CredentialRef); err != nil {
		t.Fatal("destination removal retired the shared token", err)
	}
}
