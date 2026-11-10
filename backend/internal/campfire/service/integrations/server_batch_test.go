package integrations

import (
	"errors"
	"testing"

	"campfire/internal/campfire/database/repository"
	fixture "campfire/internal/campfire/devfixture/integrations"
	core "campfire/internal/campfire/integrations"
	"campfire/internal/shared/permissions"
)

func bindingInput(value core.Binding) ServerIntegrationInput {
	return ServerIntegrationInput{IntegrationID: value.IntegrationID, Enabled: value.Enabled, ChannelID: value.DestinationID, Subscriptions: value.Subscriptions, Card: value.Card}
}

func TestIntegrationBatchSwapsChannelsAndPreservesOldCardReferences(t *testing.T) {
	db, setup, store, firstID, _ := cardService(t, fixture.DiscordHandler())
	first := cardBinding(t, store, firstID)

	second, err := setup.SaveServerSettings(t.Context(), first.ServerID, "", permissions.All(), ServerIntegrationInput{Enabled: true, ChannelID: "402"})
	if err != nil {
		t.Fatal(err)
	}

	ref := core.MessageReference{DestinationID: "400", ID: "saved-card"}
	if err := seedIntegrationCardReference(t.Context(), db, firstID, ref); err != nil {
		t.Fatal(err)
	}

	a := bindingInput(first)
	a.ChannelID = "402"
	b := ServerIntegrationInput{Enabled: true, ChannelID: "400"}

	changes := ServerIntegrationChanges{Upserts: []ServerIntegrationChange{{ID: firstID, Settings: a}, {ID: second.ID, Settings: b}}}
	for range 2 {
		values, err := setup.SaveServerIntegrationChanges(t.Context(), first.ServerID, permissions.All(), changes)
		if err != nil || len(values) != 2 {
			t.Fatal(values, err)
		}

		if current := cardBinding(t, store, firstID); current.DestinationID != "402" || current.StatusMessage != ref {
			t.Fatal("swap lost old card ownership", current)
		}

		if current := cardBinding(t, store, second.ID); current.DestinationID != "400" {
			t.Fatal("swap did not move second entry", current)
		}
	}
}

func TestIntegrationBatchRetriesCommittedCreatesAndRemovals(t *testing.T) {
	_, setup, store, original := deliveryService(t, fixture.DiscordHandler())
	binding := cardBinding(t, store, original)

	changes := ServerIntegrationChanges{
		RemoveIDs: []string{original},
		Upserts:   []ServerIntegrationChange{{ID: "stable-created-one", Settings: ServerIntegrationInput{Enabled: true, ChannelID: "400"}}, {ID: "stable-created-two", Settings: ServerIntegrationInput{Enabled: true, ChannelID: "402"}}},
	}
	for range 2 {
		result, err := setup.SaveServerIntegrationChanges(t.Context(), binding.ServerID, permissions.All(), changes)
		if err != nil || len(result) != 2 {
			t.Fatal(result, err)
		}

		items, err := store.ListBindings(t.Context(), binding.ServerID)
		if err != nil || len(items) != 2 {
			t.Fatal("retry duplicated or lost entries", items, err)
		}

		if _, err := store.GetBinding(t.Context(), original); !errors.Is(err, repository.ErrIntegrationNotFound) {
			t.Fatal("removed entry survived", err)
		}
	}

	if err := setup.RemoveServerIntegration(t.Context(), binding.ServerID, original, permissions.All()); err != nil {
		t.Fatal("completed delete was not idempotent", err)
	}
}

func TestIntegrationBatchRollsBackRemovalsAndNewRowsWhenAChannelConflicts(t *testing.T) {
	db, setup, store, original := deliveryService(t, fixture.DiscordHandler())

	binding := cardBinding(t, store, original)
	if _, err := setup.SaveServerSettings(t.Context(), binding.ServerID, "", permissions.All(), ServerIntegrationInput{Enabled: true, ChannelID: "402"}); err != nil {
		t.Fatal(err)
	}

	messageID := queueNotification(t, db, store, original, "retain-on-rollback")

	_, err := setup.SaveServerIntegrationChanges(t.Context(), binding.ServerID, permissions.All(), ServerIntegrationChanges{
		RemoveIDs: []string{original}, Upserts: []ServerIntegrationChange{{ID: "conflicting-new-entry", Settings: ServerIntegrationInput{Enabled: true, ChannelID: "402"}}},
	})
	if !errors.Is(err, repository.ErrIntegrationConflict) {
		t.Fatal("channel conflict was not rejected", err)
	}

	if _, err := store.GetBinding(t.Context(), original); err != nil {
		t.Fatal("failed batch deleted an entry", err)
	}

	if _, err := readIntegrationMessage(t.Context(), db, messageID); err != nil {
		t.Fatal("failed batch deleted pending work", err)
	}

	if _, err := store.GetBinding(t.Context(), "conflicting-new-entry"); !errors.Is(err, repository.ErrIntegrationNotFound) {
		t.Fatal("failed batch left a new row", err)
	}
}

func TestIntegrationBatchPreservesServerAndSourceAuthorization(t *testing.T) {
	_, setup, store, id := deliveryService(t, fixture.DiscordHandler())
	binding := cardBinding(t, store, id)
	input := bindingInput(binding)

	changes := ServerIntegrationChanges{Upserts: []ServerIntegrationChange{{ID: id, Settings: input}}}
	if _, err := setup.SaveServerIntegrationChanges(t.Context(), binding.ServerID, nil, changes); !errors.Is(err, repository.ErrAccessDenied) {
		t.Fatal("batch bypassed configure permission", err)
	}

	if _, err := setup.SaveServerIntegrationChanges(t.Context(), "second", permissions.All(), changes); !errors.Is(err, repository.ErrIntegrationNotFound) {
		t.Fatal("batch crossed servers", err)
	}

	if _, err := setup.SaveServerIntegrationChanges(t.Context(), "second", permissions.All(), ServerIntegrationChanges{RemoveIDs: []string{id}}); !errors.Is(err, repository.ErrIntegrationNotFound) {
		t.Fatal("batch removal crossed servers", err)
	}

	changes.Upserts[0].Settings.Subscriptions = []core.Subscription{{Category: core.BackupEvents}}
	if _, err := setup.SaveServerIntegrationChanges(t.Context(), binding.ServerID, []permissions.ID{permissions.ServerView, permissions.ServerConfigure}, changes); !errors.Is(err, repository.ErrAccessDenied) {
		t.Fatal("batch bypassed source permission", err)
	}

	changes.Upserts[0].Settings = input

	changes.RemoveIDs = []string{id}
	if _, err := setup.SaveServerIntegrationChanges(t.Context(), binding.ServerID, permissions.All(), changes); !errors.Is(err, ErrIntegrationInput) {
		t.Fatal("overlapping operations were accepted", err)
	}
}
