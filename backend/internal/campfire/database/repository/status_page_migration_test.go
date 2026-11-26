package repository_test

import (
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/integrations"
	"campfire/internal/campfire/statuspage"
)

func TestSchemaThirteenPreservesExistingServerCards(t *testing.T) {
	ctx := t.Context()

	db, original, path := integrationStore(t)
	if claim, err := original.ClaimCard(ctx, time.Now()); err != nil || claim == nil {
		t.Fatal("missing existing card work", claim, err)
	}

	ref := integrations.MessageReference{DestinationID: "channel", ID: "existing-message"}
	if err := seedIntegrationCardReference(ctx, db, "binding", ref); err != nil {
		t.Fatal(err)
	}

	var previous database.IntegrationMessage
	if err := db.ORM().Where("binding_id = ?", "binding").Take(&previous).Error; err != nil {
		t.Fatal(err)
	}

	for _, statement := range []string{
		"DROP INDEX idx_server_integrations_status_page_id",
		"ALTER TABLE server_integrations DROP COLUMN status_page_id",
		"ALTER TABLE server_integrations DROP COLUMN deleted_page_title",
		"ALTER TABLE server_integrations ALTER COLUMN server_id SET NOT NULL",
		"UPDATE schema_migrations SET version = 12",
	} {
		if err := db.ORM().Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	upgraded, err := database.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = upgraded.Close() })

	store := repository.NewIntegrationRepository(upgraded.ORM())

	old, err := store.GetBinding(ctx, "binding")
	if err != nil || old.ServerID != "first" || old.StatusMessage != ref {
		t.Fatal("lost existing server card", old, err)
	}

	var preserved database.IntegrationMessage
	if err := upgraded.ORM().First(&preserved, previous.ID).Error; err != nil || string(preserved.Payload) != string(previous.Payload) {
		t.Fatal("lost card work during migration", preserved, err)
	}

	connection, err := store.GetConnection(ctx, old.IntegrationID)
	if err != nil {
		t.Fatal(err)
	}

	page, err := repository.NewStatusPageRepository(upgraded.ORM()).SaveWithDiscord(ctx, "", statuspage.Input{
		Title: "Community", Slug: "community", DiscordChannelID: &old.DestinationID,
	}, connection)
	if err != nil {
		t.Fatal(err)
	}

	binding, err := store.PageBinding(ctx, page.ID)
	if err != nil || binding.ServerID != "" || binding.ID == old.ID {
		t.Fatal("page card did not coexist with the existing server card", binding, err)
	}
}

func TestSchemaFiveAddsStatusPagesOnUpgrade(t *testing.T) {
	path := testDatabaseSource(t)

	db, err := database.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Migrator().DropTable(&database.StatusPageEntry{}, &database.StatusPage{}); err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Table("schema_migrations").Where("version = ?", 13).Update("version", 5).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = database.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	for _, row := range []any{&database.StatusPage{}, &database.StatusPageEntry{}} {
		if !db.ORM().Migrator().HasTable(row) {
			t.Fatal("missing new status page table")
		}
	}

	var version int
	if err := db.ORM().Table("schema_migrations").Select("version").Scan(&version).Error; err != nil || version != 13 {
		t.Fatal(version, err)
	}
}
