package repository_test

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	core "campfire/internal/campfire/integrations"
)

func TestCardPollingUsesCategoryIndex(t *testing.T) {
	db, _, _ := integrationStore(t)
	if !db.ORM().Migrator().HasIndex(&database.IntegrationMessage{}, "integration_message_category_due") ||
		!db.ORM().Migrator().HasIndex(&database.IntegrationMessage{}, "integration_message_retention") {
		t.Fatal("integration maintenance indexes are missing")
	}

	if db.ORM().Name() != "sqlite" {
		return
	}

	var plan []struct{ Detail string }
	if err := db.ORM().Raw("EXPLAIN QUERY PLAN SELECT binding_id FROM integration_messages WHERE category = ? AND next_attempt_at <= ?", core.CardCategory, time.Now().Unix()).Scan(&plan).Error; err != nil {
		t.Fatal(err)
	}

	for _, step := range plan {
		if strings.Contains(step.Detail, "integration_message_category_due") {
			return
		}
	}

	t.Fatal("card polling scans notification history", plan)
}

func TestCardClaimsRecoverCreationAcrossReopenAndFenceStaleWorkers(t *testing.T) {
	db, store, source := integrationStore(t)
	now := time.Now()

	var mu sync.Mutex

	var wg sync.WaitGroup

	var claims []*repository.CardClaim

	for range 8 {
		wg.Go(func() {
			claim, err := store.ClaimCard(t.Context(), now)
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
		t.Fatal("concurrent card claims", len(claims))
	}

	claim := *claims[0]

	enqueueDelivery(t, store, "binding", "game:waiting")

	if notification, err := store.ClaimMessage(t.Context(), now); err != nil || notification != nil {
		t.Fatal("card and notification shared a channel concurrently", notification, err)
	}

	if err := store.BeginCardCreate(t.Context(), &claim, now); err != nil || !claim.Work.Creating || claim.Work.CreationID == "" {
		t.Fatal("creation identity was not persisted before sending", claim, err)
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

	recovered, err := store.ClaimCard(t.Context(), now.Add(core.DeliveryLease+time.Second))
	if err != nil || recovered == nil || !recovered.Work.Creating || recovered.Work.CreationID != claim.Work.CreationID {
		t.Fatal("lost creation identity", recovered, err)
	}

	if err := store.BeginCardCreate(t.Context(), recovered, now); !errors.Is(err, repository.ErrIntegrationConflict) {
		t.Fatal("uncertain creation permitted a replacement POST", err)
	}

	if err := store.FinishCard(t.Context(), claim, claim.Work, core.MessageReference{}, core.CardReady, "", now); !errors.Is(err, repository.ErrIntegrationConflict) {
		t.Fatal("stale owner completed card work", err)
	}

	binding, err := store.GetBinding(t.Context(), "binding")
	if err != nil {
		t.Fatal(err)
	}

	binding.DestinationID = "new-channel"
	if err := store.UpdateBinding(t.Context(), binding); err != nil {
		t.Fatal(err)
	}

	work := recovered.Work
	work.Creating = false

	ref := core.MessageReference{DestinationID: recovered.DestinationID, ID: "500"}
	if err := store.FinishCard(t.Context(), *recovered, work, ref, core.CardReady, "", now); err != nil {
		t.Fatal("lost receipt could not be adopted for old-channel cleanup", err)
	}

	cleanup, err := store.ClaimCard(t.Context(), now.Add(core.DeliveryLease+2*time.Second))
	if err != nil || cleanup == nil || cleanup.DestinationID != ref.DestinationID || cleanup.Binding.DestinationID != "new-channel" {
		t.Fatal("old destination ownership was discarded", cleanup, err)
	}

	if err := store.CheckCardMutation(t.Context(), *cleanup, true); err != nil {
		t.Fatal(err)
	}

	if err := store.FinishCard(t.Context(), *cleanup, core.CardWork{Version: 1}, core.MessageReference{}, core.CardReady, "", now); err != nil {
		t.Fatal(err)
	}

	next, err := store.ClaimCard(t.Context(), now.Add(core.DeliveryLease+3*time.Second))
	if err != nil || next == nil || next.DestinationID != "new-channel" || next.Work.Creating || next.Binding.StatusMessage.ID != "" {
		t.Fatal("new channel was not ready after cleanup", next, err)
	}
}

func TestCardDisableBeforeCreateAndNotificationWorkerSeparation(t *testing.T) {
	_, store, _ := integrationStore(t)
	now := time.Now()

	claim, err := store.ClaimCard(t.Context(), now)
	if err != nil || claim == nil {
		t.Fatal(claim, err)
	}

	binding := claim.Binding

	binding.Card.Enabled = false
	if err := store.UpdateBinding(t.Context(), binding); err != nil {
		t.Fatal(err)
	}

	if err := store.BeginCardCreate(t.Context(), claim, now); !errors.Is(err, repository.ErrIntegrationConflict) {
		t.Fatal("disabled card began sending", err)
	}

	if claim.Work.Creating {
		t.Fatal("rejected create became ambiguous")
	}

	if err := store.FinishCard(t.Context(), *claim, claim.Work, core.MessageReference{}, core.MessageGlobalLimited, "cooldown", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}

	enqueueDelivery(t, store, "binding", "game:rate-limited")

	if notification, err := store.ClaimMessage(t.Context(), now); err != nil || notification != nil {
		t.Fatal("card global cooldown did not block notifications", notification, err)
	}

	notification, err := store.ClaimMessage(t.Context(), now.Add(2*time.Minute))
	if err != nil || notification == nil || notification.Category == core.CardCategory {
		t.Fatal("notification worker claimed card control work", notification, err)
	}
}

func TestCardCleanupUsesOldDestinationWhenNewDestinationIsBlocked(t *testing.T) {
	db, store, _ := integrationStore(t)
	ctx := t.Context()

	ref := core.MessageReference{DestinationID: "channel", ID: "saved-card"}
	if err := seedIntegrationCardReference(ctx, db, "binding", ref); err != nil {
		t.Fatal(err)
	}

	if err := store.CreateBinding(ctx, core.Binding{ID: "blocked-binding", IntegrationID: "chat", ServerID: "second", DestinationID: "blocked-channel", Enabled: true}); err != nil {
		t.Fatal(err)
	}

	message := enqueueDelivery(t, store, "blocked-binding", "blocked-event")
	if err := db.ORM().Model(&database.IntegrationMessage{}).Where("id = ?", message.ID).
		Updates(map[string]any{"state": core.MessageDestinationBlocked, "next_attempt_at": 0}).Error; err != nil {
		t.Fatal(err)
	}

	binding, err := store.GetBinding(ctx, "binding")
	if err != nil {
		t.Fatal(err)
	}

	binding.DestinationID, binding.Enabled = "blocked-channel", false
	if err := store.UpdateBinding(ctx, binding); err != nil {
		t.Fatal(err)
	}

	claim, err := store.ClaimCard(ctx, time.Now())
	if err != nil || claim == nil || claim.DestinationID != ref.DestinationID {
		t.Fatal("cleanup was gated by the new destination", claim, err)
	}
}
