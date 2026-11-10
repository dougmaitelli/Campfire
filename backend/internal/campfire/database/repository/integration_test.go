package repository_test

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/integrations"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

func integrationStore(t *testing.T) (*database.Database, *repository.IntegrationRepository, string) {
	t.Helper()

	source := testDatabaseSource(t)

	db, err := database.Open(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	for _, row := range []any{
		&database.Spark{ID: "node", Name: "Node"},
		&database.Ember{ID: "game", Name: "Game", Definition: model.Ember{Schema: 1}},
		&database.Server{ID: "first", Name: "First", SparkID: "node", EmberID: "game", Variables: map[string]string{"KEEP": "value"}},
		&database.Server{ID: "second", Name: "Second", SparkID: "node", EmberID: "game", Variables: map[string]string{}},
	} {
		if err := db.ORM().Omit("Spark", "Ember", "Owner").Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}

	store := repository.NewIntegrationRepository(db.ORM())

	connection := integrations.Connection{ID: "chat", Provider: "discord", Name: "Community", Enabled: true, Config: json.RawMessage(`{"guild":"community"}`), CredentialRef: "secrets/chat"}
	if err := store.CreateConnection(t.Context(), connection); err != nil {
		t.Fatal(err)
	}

	binding := integrations.Binding{ID: "binding", IntegrationID: connection.ID, ServerID: "first", DestinationID: "channel", Enabled: true, Subscriptions: []integrations.Subscription{{Category: integrations.GameEvents, Rules: []integrations.EventRule{{Type: "player.joined", Fields: map[string]string{"state": "ready"}}}}}, Card: integrations.CardSettings{Enabled: true, Fields: []string{"players", "fields.invite_code"}}}
	if err := store.CreateBinding(t.Context(), binding); err != nil {
		t.Fatal(err)
	}

	return db, store, source
}

func TestIntegrationConnectionsAndBindingsAreProviderIndependent(t *testing.T) {
	_, store, _ := integrationStore(t)
	ctx := t.Context()

	second := integrations.Connection{ID: "other", Provider: "test-provider", Name: "Other provider", Enabled: true, Config: json.RawMessage(`{"workspace":"team"}`)}
	if err := store.CreateConnection(ctx, second); err != nil {
		t.Fatal(err)
	}

	connections, err := store.ListConnections(ctx)
	if err != nil || len(connections) != 2 || connections[1].Provider != second.Provider {
		t.Fatal("provider connections were restricted or lost", connections, err)
	}

	binding, err := store.GetBinding(ctx, "binding")
	if err != nil || binding.Subscriptions[0].Rules[0].Fields["state"] != "ready" || len(binding.Card.Fields) != 2 {
		t.Fatal("subscription or card settings were lost", binding, err)
	}

	duplicate := binding

	duplicate.ID = "duplicate"
	if err := store.CreateBinding(ctx, duplicate); !errors.Is(err, repository.ErrIntegrationConflict) {
		t.Fatal("duplicate connection/server binding was accepted", err)
	}

	for _, next := range []integrations.Binding{
		{ID: "other-provider", IntegrationID: second.ID, ServerID: "first", DestinationID: "room", Enabled: true},
		{ID: "shared-channel", IntegrationID: "chat", ServerID: "second", DestinationID: binding.DestinationID, Enabled: true},
	} {
		if err := store.CreateBinding(ctx, next); err != nil {
			t.Fatal("multiple providers or a shared channel were rejected", err)
		}
	}

	bindings, err := store.ListBindings(ctx, "first")
	if err != nil || len(bindings) != 2 {
		t.Fatal("server binding list crossed server boundaries", bindings, err)
	}

	connection, err := store.GetConnection(ctx, "chat")
	if err != nil {
		t.Fatal(err)
	}

	encoded, err := json.Marshal(connection)
	if err != nil || strings.Contains(string(encoded), connection.CredentialRef) || strings.Contains(string(encoded), "credential") {
		t.Fatal("secret reference leaked into JSON", string(encoded), err)
	}

	created := connection.CreatedAt

	connection.Name, connection.Enabled, connection.Config = "Renamed", false, json.RawMessage(`{"guild":"new"}`)
	if err := store.UpdateConnection(ctx, connection); err != nil {
		t.Fatal(err)
	}

	connection, err = store.GetConnection(ctx, connection.ID)
	if err != nil || connection.Enabled || connection.Name != "Renamed" || connection.CreatedAt != created || string(connection.Config) != `{"guild":"new"}` {
		t.Fatal("connection settings were not updated independently", connection, err)
	}
}

func TestIntegrationCardReferencesSurviveConfigurationChanges(t *testing.T) {
	db, store, _ := integrationStore(t)
	ctx := t.Context()
	empty := integrations.MessageReference{}

	original := integrations.MessageReference{DestinationID: "channel", ID: "external-card"}
	if err := seedIntegrationCardReference(ctx, db, "binding", original); err != nil {
		t.Fatal(err)
	}

	binding, err := store.GetBinding(ctx, "binding")
	if err != nil {
		t.Fatal(err)
	}

	binding.DestinationID, binding.Enabled, binding.Subscriptions, binding.Card = "new-channel", false, nil, integrations.CardSettings{}

	binding.StatusMessage = empty // A settings form must not overwrite reconciliation state.
	if err := store.UpdateBinding(ctx, binding); err != nil {
		t.Fatal(err)
	}

	binding, err = store.GetBinding(ctx, binding.ID)
	if err != nil || binding.Enabled || binding.Card.Enabled || len(binding.Subscriptions) != 0 || binding.StatusMessage != original {
		t.Fatal("settings update lost the old card or ignored false values", binding, err)
	}

	claim, err := store.ClaimCard(ctx, time.Now())
	if err != nil || claim == nil || claim.DestinationID != original.DestinationID {
		t.Fatal("cleanup lost the old channel", claim, err)
	}

	if err := store.CheckCardMutation(ctx, *claim, true); err != nil {
		t.Fatal(err)
	}

	if err := store.FinishCard(ctx, *claim, integrations.CardWork{Version: 1}, empty, integrations.CardReady, "", time.Time{}); err != nil {
		t.Fatal(err)
	}

	binding, err = store.GetBinding(ctx, binding.ID)
	if err != nil || binding.StatusMessage != empty {
		t.Fatal("cleanup did not clear the card reference", binding, err)
	}
}

func TestIntegrationMessagesAreImmutableDeduplicatedSnapshots(t *testing.T) {
	db, store, _ := integrationStore(t)
	ctx := t.Context()
	now := time.Unix(1800000000, 0)
	payload := json.RawMessage(`{"title":"Player joined","player":"Alex"}`)

	first, err := store.EnqueueMessage(ctx, "binding", "game:event-1", integrations.GameEvents, payload, now)
	if err != nil || first.ID == 0 || first.State != model.DeliveryPending || first.Attempts != 0 {
		t.Fatal("outgoing message was not persisted pending", first, err)
	}

	if err := db.ORM().Model(&database.IntegrationMessage{}).Where("id = ?", first.ID).
		Updates(map[string]any{"state": model.DeliveryDelivered, "attempts": 2, "external_message_id": "sent", "delivered_at": now.Unix()}).Error; err != nil {
		t.Fatal(err)
	}

	binding, err := store.GetBinding(ctx, "binding")
	if err != nil {
		t.Fatal(err)
	}

	binding.DestinationID = "new-channel"
	if err := store.UpdateBinding(ctx, binding); err != nil {
		t.Fatal(err)
	}

	replayed, err := store.EnqueueMessage(ctx, binding.ID, first.EventID, integrations.GameEvents, json.RawMessage(`{"title":"Changed"}`), now.Add(time.Hour))
	if err != nil || replayed.ID != first.ID || replayed.State != model.DeliveryDelivered || replayed.Attempts != 2 || replayed.ExternalMessageID != "sent" || replayed.DestinationID != "channel" || string(replayed.Payload) != string(payload) || replayed.CreatedAt != now.Unix() {
		t.Fatal("replay changed the original message or reset delivery state", replayed, err)
	}

	next, err := store.EnqueueMessage(ctx, binding.ID, "game:event-2", integrations.GameEvents, payload, now)
	if err != nil || next.DestinationID != binding.DestinationID {
		t.Fatal("new message did not snapshot the new destination", next, err)
	}

	connection, err := store.GetConnection(ctx, "chat")
	if err != nil {
		t.Fatal(err)
	}

	connection.Enabled = false
	if err := store.UpdateConnection(ctx, connection); err != nil {
		t.Fatal(err)
	}

	if _, err := store.EnqueueMessage(ctx, binding.ID, "disabled", integrations.GameEvents, payload, now); !errors.Is(err, repository.ErrIntegrationDisabled) {
		t.Fatal("disabled connection accepted a new message", err)
	}

	connection.Enabled, binding.Enabled = true, false
	if err := store.UpdateConnection(ctx, connection); err != nil {
		t.Fatal(err)
	}

	if err := store.UpdateBinding(ctx, binding); err != nil {
		t.Fatal(err)
	}

	if _, err := store.EnqueueMessage(ctx, binding.ID, "disabled-binding", integrations.GameEvents, payload, now); !errors.Is(err, repository.ErrIntegrationDisabled) {
		t.Fatal("disabled binding accepted a new message", err)
	}
}

func TestIntegrationMessagesJoinProducerTransactionsAndSerializeReplays(t *testing.T) {
	db, store, _ := integrationStore(t)
	ctx := t.Context()
	payload := json.RawMessage(`{"title":"Backup complete"}`)
	aborted := errors.New("abort producer")

	err := database.Transaction(db.ORM().WithContext(ctx), func(tx *gorm.DB) error {
		if _, err := repository.NewIntegrationRepository(tx).EnqueueMessage(ctx, "binding", "rolled-back", integrations.BackupEvents, payload, time.Now()); err != nil {
			return err
		}

		return aborted
	})
	if !errors.Is(err, aborted) {
		t.Fatal(err)
	}

	var count int64
	if err := db.ORM().Model(&database.IntegrationMessage{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("producer rollback left an outgoing message", count, err)
	}

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, err := store.EnqueueMessage(ctx, "binding", "concurrent", integrations.BackupEvents, payload, time.Now()); err != nil {
				t.Error(err)
			}
		})
	}

	wg.Wait()

	if err := db.ORM().Model(&database.IntegrationMessage{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("concurrent replay created duplicate messages", count, err)
	}
}

func TestIntegrationForeignKeysAndCascades(t *testing.T) {
	db, store, _ := integrationStore(t)
	ctx := t.Context()

	missing := integrations.Binding{ID: "missing", IntegrationID: "chat", ServerID: "absent", DestinationID: "channel"}
	if err := store.CreateBinding(ctx, missing); !errors.Is(err, gorm.ErrForeignKeyViolated) {
		t.Fatal("binding accepted an unknown server", err)
	}

	missing.ServerID, missing.IntegrationID = "first", "absent"
	if err := store.CreateBinding(ctx, missing); !errors.Is(err, gorm.ErrForeignKeyViolated) {
		t.Fatal("binding accepted an unknown connection", err)
	}

	message, err := store.EnqueueMessage(ctx, "binding", "event", integrations.PowerEvents, json.RawMessage(`{"state":"running"}`), time.Now())
	if err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Where("id = ?", "first").Delete(&database.Server{}).Error; err != nil {
		t.Fatal(err)
	}

	if _, err := store.GetBinding(ctx, "binding"); !errors.Is(err, repository.ErrIntegrationNotFound) {
		t.Fatal("server deletion left a binding", err)
	}

	if _, err := readIntegrationMessage(ctx, db, message.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatal("server deletion left an outgoing message", err)
	}

	if _, err := store.GetConnection(ctx, "chat"); err != nil {
		t.Fatal("server deletion removed a shared connection", err)
	}

	missing.ID, missing.IntegrationID, missing.ServerID = "second-binding", "chat", "second"
	if err := store.CreateBinding(ctx, missing); err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Where("id = ?", "chat").Delete(&database.Integration{}).Error; err != nil {
		t.Fatal(err)
	}

	if _, err := store.GetBinding(ctx, missing.ID); !errors.Is(err, repository.ErrIntegrationNotFound) {
		t.Fatal("connection deletion left a binding", err)
	}
}

func TestIntegrationInvalidConfigurationCannotEnterStorage(t *testing.T) {
	db, store, _ := integrationStore(t)
	ctx := t.Context()

	for _, config := range []json.RawMessage{nil, json.RawMessage(`null`), json.RawMessage(`[]`), json.RawMessage(`{"broken"`)} {
		value := integrations.Connection{ID: "invalid", Provider: "test-provider", Name: "Invalid", Config: config}
		if err := store.CreateConnection(ctx, value); err == nil {
			t.Fatal("invalid provider configuration was persisted", string(config))
		}
	}

	binding, err := store.GetBinding(ctx, "binding")
	if err != nil {
		t.Fatal(err)
	}

	binding.Subscriptions = append(binding.Subscriptions, binding.Subscriptions[0])
	if err := store.UpdateBinding(ctx, binding); err == nil {
		t.Fatal("duplicate category subscriptions were persisted")
	}

	for _, payload := range []json.RawMessage{nil, json.RawMessage(`null`), json.RawMessage(`[]`), json.RawMessage(`{"broken"`)} {
		if _, err := store.EnqueueMessage(ctx, "binding", "invalid", integrations.GameEvents, payload, time.Now()); err == nil {
			t.Fatal("invalid outgoing payload was persisted", string(payload))
		}
	}

	var count int64
	if err := db.ORM().Model(&database.IntegrationMessage{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("invalid input left an outgoing message", count, err)
	}

	binding, err = store.GetBinding(ctx, binding.ID)
	if err != nil || len(binding.Subscriptions) != 1 || binding.StatusMessage != (integrations.MessageReference{}) {
		t.Fatal("invalid input changed existing settings", binding, err)
	}
}

func TestIntegrationAdditiveMigrationAndReopenPreserveExistingData(t *testing.T) {
	db, _, source := integrationStore(t)
	ctx := t.Context()

	for _, table := range []any{&database.IntegrationMessage{}, &database.ServerIntegration{}, &database.Integration{}} {
		if err := db.ORM().Migrator().DropTable(table); err != nil {
			t.Fatal(err)
		}
	}

	user := database.User{Username: "existing", PasswordHash: "unchanged", CredentialVersion: 7, RoleID: database.AdminRole}
	if err := db.ORM().Omit("Role").Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	upgraded, err := database.Open(ctx, source)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = upgraded.Close() })

	for _, table := range []any{&database.Integration{}, &database.ServerIntegration{}, &database.IntegrationMessage{}} {
		if !upgraded.ORM().Migrator().HasTable(table) {
			t.Fatalf("upgrade did not create %T", table)
		}
	}

	var preserved database.User
	if err := upgraded.ORM().Where("id = ?", user.ID).Take(&preserved).Error; err != nil || preserved.PasswordHash != user.PasswordHash || preserved.CredentialVersion != user.CredentialVersion {
		t.Fatal("additive migration changed existing credentials", err)
	}

	var server database.Server
	if err := upgraded.ORM().Where("id = ?", "first").Take(&server).Error; err != nil || server.Variables["KEEP"] != "value" {
		t.Fatal("additive migration changed server configuration", err)
	}

	store := repository.NewIntegrationRepository(upgraded.ORM())
	if err := store.CreateConnection(ctx, integrations.Connection{ID: "persisted", Provider: "test-provider", Name: "Persisted", Enabled: true, Config: json.RawMessage(`{}`)}); err != nil {
		t.Fatal(err)
	}

	if err := store.CreateBinding(ctx, integrations.Binding{ID: "persisted-binding", IntegrationID: "persisted", ServerID: "first", DestinationID: "room", Enabled: true}); err != nil {
		t.Fatal(err)
	}

	ref := integrations.MessageReference{DestinationID: "room", ID: "card"}
	if err := seedIntegrationCardReference(ctx, upgraded, "persisted-binding", ref); err != nil {
		t.Fatal(err)
	}

	message, err := store.EnqueueMessage(ctx, "persisted-binding", "retained", integrations.PowerEvents, json.RawMessage(`{"state":"stopped"}`), time.Now())
	if err != nil {
		t.Fatal(err)
	}

	if err := upgraded.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := database.Open(ctx, source)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = reopened.Close() })

	store = repository.NewIntegrationRepository(reopened.ORM())

	binding, err := store.GetBinding(ctx, "persisted-binding")
	if err != nil || binding.StatusMessage != ref {
		t.Fatal("reopen lost persistent card identity", binding, err)
	}

	if retained, err := readIntegrationMessage(ctx, reopened, message.ID); err != nil || retained.EventID != message.EventID || retained.State != model.DeliveryPending {
		t.Fatal("reopen lost the pending message", retained, err)
	}
}
