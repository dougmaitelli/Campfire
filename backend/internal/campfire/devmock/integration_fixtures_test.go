//go:build !production

package devmock

import (
	"path/filepath"
	"testing"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/devmock/integrations"
)

func TestMockDiscordStartsConnectedAndPreservesEditsAndRemoval(t *testing.T) {
	runtime, db, _, _ := fixture(t)

	provider := integrations.NewDiscord()
	defer provider.Close()

	ctx := t.Context()

	directory := filepath.Join(t.TempDir(), "integration-secrets")
	if err := runtime.EnableIntegrationExample(ctx, provider, directory); err != nil {
		t.Fatal(err)
	}

	store := repository.NewIntegrationRepository(db.ORM())

	connections, err := store.ListConnections(ctx)
	if err != nil || len(connections) != 1 || !connections[0].Enabled {
		t.Fatal("mock bot is not connected", connections, err)
	}

	bindings, err := store.ListBindings(ctx, "survival")
	if err != nil || len(bindings) != 1 || !bindings[0].Enabled || !bindings[0].Card.Enabled || bindings[0].DestinationID != "400" {
		t.Fatal("Survival has no working example", bindings, err)
	}

	connection := connections[0]

	connection.Enabled = false
	if err := store.UpdateConnection(ctx, connection); err != nil {
		t.Fatal(err)
	}

	if err := runtime.EnableIntegrationExample(ctx, provider, directory); err != nil {
		t.Fatal(err)
	}

	saved, err := store.GetConnection(ctx, connection.ID)
	if err != nil || saved.Enabled || saved.CredentialRef != connection.CredentialRef {
		t.Fatal("rebuild overwrote connection edits", saved, err)
	}

	if err := store.DeleteConnection(ctx, connection.ID); err != nil {
		t.Fatal(err)
	}

	if err := runtime.EnableIntegrationExample(ctx, provider, directory); err != nil {
		t.Fatal(err)
	}

	connections, err = store.ListConnections(ctx)
	if err != nil || len(connections) != 0 {
		t.Fatal("rebuild restored a removed connection", connections, err)
	}
}
