package repository_test

import (
	"testing"

	"campfire/internal/campfire/integrations"
	setup "campfire/internal/campfire/service/integrations"
	"campfire/internal/shared/permissions"
)

// The repository suite runs against SQLite and PostgreSQL in CI. Exercise the
// service transaction here so destination swaps cover both unique-index engines.
func TestIntegrationBatchChannelSwapOnDatabase(t *testing.T) {
	db, store, _ := integrationStore(t)
	ctx := t.Context()

	first, err := store.GetBinding(ctx, "binding")
	if err != nil {
		t.Fatal(err)
	}

	first.Enabled = false

	first.DestinationID = "400"
	if err := store.UpdateBinding(ctx, first); err != nil {
		t.Fatal(err)
	}

	second := first
	second.ID = "second-destination"

	second.DestinationID = "402"
	if err := store.CreateBinding(ctx, second); err != nil {
		t.Fatal(err)
	}

	ref := integrations.MessageReference{DestinationID: "400", ID: "old-card"}
	if err := seedIntegrationCardReference(ctx, db, first.ID, ref); err != nil {
		t.Fatal(err)
	}

	service := setup.NewDiscordIntegrationService(db.ORM(), setup.Secrets{}, nil)

	changes := setup.ServerIntegrationChanges{Upserts: []setup.ServerIntegrationChange{
		{ID: first.ID, Settings: setup.ServerIntegrationInput{IntegrationID: first.IntegrationID, ChannelID: "402"}},
		{ID: second.ID, Settings: setup.ServerIntegrationInput{IntegrationID: first.IntegrationID, ChannelID: "400"}},
	}}
	for range 2 {
		if _, err := service.SaveServerIntegrationChanges(ctx, first.ServerID, permissions.All(), changes); err != nil {
			t.Fatal(err)
		}

		moved, err := store.GetBinding(ctx, first.ID)
		if err != nil || moved.DestinationID != "402" || moved.StatusMessage != ref {
			t.Fatal("swap lost its destination or cleanup reference", moved, err)
		}

		other, err := store.GetBinding(ctx, second.ID)
		if err != nil || other.DestinationID != "400" {
			t.Fatal("swap lost the second destination", other, err)
		}
	}
}
