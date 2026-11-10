package repository_test

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	core "campfire/internal/campfire/integrations"
	"campfire/internal/shared/model"
)

func enqueueDelivery(t *testing.T, store *repository.IntegrationRepository, bindingID, eventID string) core.Message {
	t.Helper()

	binding, err := store.GetBinding(t.Context(), bindingID)
	if err != nil {
		t.Fatal(err)
	}

	payload, err := json.Marshal(core.Event{ID: eventID, ServerID: binding.ServerID, Category: core.GameEvents, Type: "player.joined", Title: "Player joined", OccurredAt: time.Now().Unix(), Data: map[string]any{"state": "ready"}})
	if err != nil {
		t.Fatal(err)
	}

	message, err := store.EnqueueMessage(t.Context(), bindingID, eventID, core.GameEvents, payload, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	return message
}

func TestIntegrationClaimsSerializeDestinationsAndFenceStaleCompletion(t *testing.T) {
	db, store, source := integrationStore(t)
	enqueueDelivery(t, store, "binding", "one")
	enqueueDelivery(t, store, "binding", "two")

	now := time.Now()

	var wg sync.WaitGroup

	var mu sync.Mutex

	var claims []*repository.IntegrationClaim

	for range 8 {
		wg.Go(func() {
			claim, err := store.ClaimMessage(t.Context(), now)
			if err != nil {
				t.Error(err)
				return
			}

			if claim != nil {
				mu.Lock()

				claims = append(claims, claim)
				mu.Unlock()
			}
		})
	}

	wg.Wait()

	if len(claims) != 1 {
		t.Fatal("destination was claimed concurrently", len(claims))
	}

	old := *claims[0]
	if err := store.BeginMessageSend(t.Context(), old); err != nil {
		t.Fatal(err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := database.Open(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = reopened.Close() })

	store = repository.NewIntegrationRepository(reopened.ORM())

	recovered, err := store.ClaimMessage(t.Context(), now.Add(core.DeliveryLease+time.Second))
	if err != nil || recovered == nil || !recovered.Reconcile || recovered.ID != old.ID {
		t.Fatal("lost send was not recovered as uncertain", recovered, err)
	}

	if err := store.FinishMessage(t.Context(), old, model.DeliveryDelivered, "stale", "", time.Time{}, now); !errors.Is(err, repository.ErrIntegrationConflict) {
		t.Fatal("stale worker replaced current lease", err)
	}

	if err := store.FinishMessage(t.Context(), *recovered, model.DeliveryDelivered, "receipt", "", time.Time{}, now); err != nil {
		t.Fatal(err)
	}

	next, err := store.ClaimMessage(t.Context(), now.Add(core.DeliveryLease+2*time.Second))
	if err != nil || next == nil || next.ID == old.ID || next.Reconcile {
		t.Fatal("completed head blocked the next event", next, err)
	}
}

func TestIntegrationClaimChecksDisablingChangesAndRateScopes(t *testing.T) {
	_, store, _ := integrationStore(t)
	enqueueDelivery(t, store, "binding", "one")

	now := time.Now()

	claim, err := store.ClaimMessage(t.Context(), now)
	if err != nil || claim == nil {
		t.Fatal(claim, err)
	}

	binding, err := store.GetBinding(t.Context(), "binding")
	if err != nil {
		t.Fatal(err)
	}

	binding.Enabled = false
	if err := store.UpdateBinding(t.Context(), binding); err != nil {
		t.Fatal(err)
	}

	if err := store.BeginMessageSend(t.Context(), *claim); !errors.Is(err, repository.ErrIntegrationDisabled) {
		t.Fatal("disabled binding allowed a post", err)
	}

	if err := store.FinishMessage(t.Context(), *claim, model.DeliveryPending, "", "", now, now); err != nil {
		t.Fatal(err)
	}

	if next, err := store.ClaimMessage(t.Context(), now); err != nil || next != nil {
		t.Fatal("disabled binding was claimed", next, err)
	}

	binding.Enabled = true
	if err := store.UpdateBinding(t.Context(), binding); err != nil {
		t.Fatal(err)
	}

	if err := store.CreateBinding(t.Context(), core.Binding{ID: "other-channel", IntegrationID: "chat", ServerID: "second", DestinationID: "other", Enabled: true}); err != nil {
		t.Fatal(err)
	}

	enqueueDelivery(t, store, "other-channel", "other")

	claim, err = store.ClaimMessage(t.Context(), now)
	if err != nil || claim == nil || claim.BindingID != "binding" {
		t.Fatal(claim, err)
	}

	deadline := now.Add(10 * time.Minute)
	if err := store.FinishMessage(t.Context(), *claim, core.MessageGlobalLimited, "", "Rate limited", deadline, now); err != nil {
		t.Fatal(err)
	}

	if next, err := store.ClaimMessage(t.Context(), now.Add(time.Minute)); err != nil || next != nil {
		t.Fatal("global cooldown did not block another channel", next, err)
	}

	claim, err = store.ClaimMessage(t.Context(), deadline.Add(time.Second))
	if err != nil || claim == nil {
		t.Fatal(claim, err)
	}

	if err := store.FinishMessage(t.Context(), *claim, core.MessageRateLimited, "", "Channel rate limited", deadline.Add(time.Minute), now); err != nil {
		t.Fatal(err)
	}

	next, err := store.ClaimMessage(t.Context(), deadline.Add(2*time.Second))
	if err != nil || next == nil || next.BindingID == claim.BindingID {
		t.Fatal("one channel's cooldown blocked another channel", next, err)
	}
}

func TestIntegrationStatusAndRetryEnforceCategoryAndDestination(t *testing.T) {
	db, store, _ := integrationStore(t)

	message := enqueueDelivery(t, store, "binding", "one")
	if err := db.ORM().Model(&database.IntegrationMessage{}).Where("id = ?", message.ID).Updates(map[string]any{"state": model.DeliveryFailed, "error": "Retry me", "next_attempt_at": 0}).Error; err != nil {
		t.Fatal(err)
	}

	if _, err := store.EnqueueMessage(t.Context(), "binding", "private", core.ConsoleAlerts, json.RawMessage(`{"title":"private guidance"}`), time.Now()); err != nil {
		t.Fatal(err)
	}

	items, err := store.MessageStatuses(t.Context(), "binding", []core.Category{core.GameEvents}, 0, "failed")
	if err != nil || len(items) != 1 || !items[0].CanRetry {
		t.Fatal(items, err)
	}

	if err := store.RetryMessage(t.Context(), "binding", message.ID, []core.Category{core.BackupEvents}, time.Now()); !errors.Is(err, repository.ErrIntegrationNotFound) {
		t.Fatal("source permission bypass", err)
	}

	binding, err := store.GetBinding(t.Context(), "binding")
	if err != nil {
		t.Fatal(err)
	}

	binding.DestinationID = "changed"
	if err := store.UpdateBinding(t.Context(), binding); err != nil {
		t.Fatal(err)
	}

	if err := store.RetryMessage(t.Context(), "binding", message.ID, []core.Category{core.GameEvents}, time.Now()); !errors.Is(err, repository.ErrIntegrationConflict) {
		t.Fatal("old channel was retried", err)
	}

	items, err = store.MessageStatuses(t.Context(), "binding", []core.Category{core.GameEvents}, 0, "failed")
	if err != nil || items[0].CanRetry {
		t.Fatal("unavailable retry remained visible", items, err)
	}
}

func TestStaleCredentialFailuresCannotPauseRotatedConnections(t *testing.T) {
	db, store, _ := integrationStore(t)
	message := enqueueDelivery(t, store, "binding", "rotate")
	now := time.Now()

	claim, err := store.ClaimMessage(t.Context(), now)
	if err != nil || claim == nil {
		t.Fatal(claim, err)
	}

	connection, err := store.GetConnection(t.Context(), "chat")
	if err != nil {
		t.Fatal(err)
	}

	connection.CredentialRef = "rotated"
	if err := store.UpdateConnection(t.Context(), connection); err != nil {
		t.Fatal(err)
	}

	if err := store.FinishMessage(t.Context(), *claim, core.MessageCredentialsBlocked, "", "Old credential rejected", time.Time{}, now); err != nil {
		t.Fatal(err)
	}

	saved, err := readIntegrationMessage(t.Context(), db, message.ID)
	if err != nil || saved.State != model.DeliveryPending || saved.NextAttemptAt <= now.Unix() {
		t.Fatal("stale credentials paused a healthy connection", saved, err)
	}
}
