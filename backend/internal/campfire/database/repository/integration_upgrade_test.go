package repository_test

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	core "campfire/internal/campfire/integrations"
	"campfire/internal/shared/model"
	"campfire/internal/shared/permissions"
)

func TestIntegrationUpgradeRemovesObsoleteGrantsWithoutPromotingEditors(t *testing.T) {
	db, _, source := integrationStore(t)

	const obsolete = "server.integrations.manage"

	role := database.Role{Name: "Former integration manager", Scope: model.RoleScopeServer,
		Permissions: []database.RolePermission{{Permission: obsolete}, {Permission: permissions.ServerView}}}
	if err := db.ORM().Create(&role).Error; err != nil {
		t.Fatal(err)
	}

	user := database.User{Username: "former-manager", PasswordHash: "preserved", RoleID: database.UserRole, CredentialVersion: 1}
	if err := db.ORM().Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	token := database.APIToken{UserID: user.ID, Name: "Former integration token", Hash: "preserved", Prefix: "cf_former", CredentialVersion: 1,
		Scopes: []model.PermissionID{permissions.ServerView, obsolete}}
	if err := db.ORM().Create(&token).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	for range 2 {
		reopened, err := database.Open(t.Context(), source)
		if err != nil {
			t.Fatal(err)
		}

		if err := reopened.ORM().Preload("Permissions").Take(&role, role.ID).Error; err != nil {
			t.Fatal(err)
		}

		if len(role.Permissions) != 1 || role.Permissions[0].Permission != permissions.ServerView {
			t.Fatal("obsolete role grant survived or acquired configure", role.Permissions)
		}

		if err := reopened.ORM().Take(&token, token.ID).Error; err != nil {
			t.Fatal(err)
		}

		if !slices.Equal(token.Scopes, []model.PermissionID{permissions.ServerView}) {
			t.Fatal("obsolete token scope survived or acquired configure", token.Scopes)
		}

		if err := reopened.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestIntegrationDestinationAndScopedRuleUpgradePreservesOwnership(t *testing.T) {
	db, store, source := integrationStore(t)
	ctx := t.Context()

	ref := core.MessageReference{DestinationID: "channel", ID: "retained-card"}
	if err := seedIntegrationCardReference(ctx, db, "binding", ref); err != nil {
		t.Fatal(err)
	}

	message, err := store.EnqueueMessage(ctx, "binding", "retained-notification", core.GameEvents, json.RawMessage(`{"title":"Retained"}`), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	// Recreate the actual previous schema and JSON, rather than using the current model.
	if err := db.ORM().Migrator().DropIndex(&database.ServerIntegration{}, "server_integration_binding"); err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Exec("CREATE UNIQUE INDEX server_integration_binding ON server_integrations (integration_id, server_id)").Error; err != nil {
		t.Fatal(err)
	}

	for _, index := range []string{"integration_message_category_due", "integration_message_retention"} {
		if err := db.ORM().Migrator().DropIndex(&database.IntegrationMessage{}, index); err != nil {
			t.Fatal(err)
		}
	}

	old := `[{"category":"game","types":["player.joined","game.announcement"],"fields":{"player":"doug"}},{"category":"power","fields":{"state":"running"}}]`
	if err := db.ORM().Table("server_integrations").Where("id = ?", "binding").Update("subscriptions", old).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := database.Open(ctx, source)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = reopened.Close() })

	for _, index := range []string{"integration_message_category_due", "integration_message_retention"} {
		if !reopened.ORM().Migrator().HasIndex(&database.IntegrationMessage{}, index) {
			t.Fatal("upgrade did not add maintenance index", index)
		}
	}

	store = repository.NewIntegrationRepository(reopened.ORM())

	binding, err := store.GetBinding(ctx, "binding")
	if err != nil || binding.StatusMessage != ref || len(binding.Subscriptions[0].Rules) != 2 {
		t.Fatal("migration lost identity or rules", binding, err)
	}

	subscription := binding.Subscriptions[0]
	if len(binding.Subscriptions[1].Rules) != 1 || binding.Subscriptions[1].Rules[0].Type != "" || binding.Subscriptions[1].Rules[0].Fields["state"] != "running" {
		t.Fatal("migration lost an all-types field filter", binding.Subscriptions[1])
	}

	if !subscription.Matches(core.Event{Category: core.GameEvents, Type: "player.joined", Data: map[string]any{"player": "Doug"}}) || subscription.Matches(core.Event{Category: core.GameEvents, Type: "game.announcement"}) {
		t.Fatal("legacy filters changed meaning", subscription)
	}

	if retained, err := readIntegrationMessage(ctx, reopened, message.ID); err != nil || retained.EventID != message.EventID {
		t.Fatal("migration lost pending work", retained, err)
	}

	second := binding

	second.ID, second.DestinationID, second.StatusMessage = "admin-destination", "admin-channel", core.MessageReference{}
	if err := store.CreateBinding(ctx, second); err != nil {
		t.Fatal("second destination is still blocked", err)
	}

	second.ID = "duplicate"
	if err := store.CreateBinding(ctx, second); !errors.Is(err, repository.ErrIntegrationConflict) {
		t.Fatal("duplicate destination accepted", err)
	}

	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}

	again, err := database.Open(ctx, source)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = again.Close() })

	bindings, err := repository.NewIntegrationRepository(again.ORM()).ListBindings(ctx, "first")
	if err != nil || len(bindings) != 2 || len(bindings[0].Subscriptions[0].Rules) != 2 {
		t.Fatal("upgrade is not repeatable", bindings, err)
	}
}
