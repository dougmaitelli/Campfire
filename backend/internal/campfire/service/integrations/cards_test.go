package integrations

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	fixture "campfire/internal/campfire/devfixture/integrations"
	core "campfire/internal/campfire/integrations"
	"campfire/internal/shared/model"
	accessperm "campfire/internal/shared/permissions"
)

type staticCardSource struct{ card core.ServerCard }

func (s *staticCardSource) IntegrationCard(context.Context, string, core.CardSettings) (core.ServerCard, error) {
	return s.card, nil
}

func cardService(t *testing.T, remote http.Handler) (*database.Database, *DiscordIntegrationService, *repository.IntegrationRepository, string, *staticCardSource) {
	t.Helper()
	db, setup, store, id := deliveryService(t, remote)

	binding, err := store.GetBinding(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}

	binding.Card = core.CardSettings{Enabled: true, Fields: []string{"players"}}

	binding.Subscriptions = nil
	if err := store.UpdateBinding(t.Context(), binding); err != nil {
		t.Fatal(err)
	}

	source := &staticCardSource{card: core.ServerCard{Name: "First", State: model.RuntimeStateRunning, Addresses: []string{"games.example.test:25565 / TCP"}, Fields: []core.CardField{{ID: "players", Name: "Players", Value: "3", Available: true}}}}
	setup.WithCards(source)

	return db, setup, store, id, source
}

func makeCardDue(t *testing.T, db *database.Database, bindingID string, check bool) {
	t.Helper()

	var row database.IntegrationMessage
	if err := db.ORM().Where("binding_id = ? AND category = ?", bindingID, core.CardCategory).Take(&row).Error; err != nil {
		t.Fatal(err)
	}

	updates := map[string]any{"next_attempt_at": 1}

	if check {
		var work core.CardWork
		if err := json.Unmarshal(row.Payload, &work); err != nil {
			t.Fatal(err)
		}

		work.CheckedAt = 0

		payload, err := json.Marshal(work)
		if err != nil {
			t.Fatal(err)
		}

		updates["payload"] = string(payload)
	}

	if err := db.ORM().Model(&row).Updates(updates).Error; err != nil {
		t.Fatal(err)
	}
}

func runCard(t *testing.T, setup *DiscordIntegrationService) {
	t.Helper()

	if err := setup.cards.RunOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func cardBinding(t *testing.T, store *repository.IntegrationRepository, id string) core.Binding {
	t.Helper()

	binding, err := store.GetBinding(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}

	return binding
}

func TestCardsCreateCoalesceEditAndRecoverConfirmedDeletion(t *testing.T) {
	base := fixture.DiscordHandler()

	var posts, patches, reads atomic.Int64

	db, setup, store, id, source := cardService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/messages") {
			switch r.Method {
			case http.MethodPost:
				posts.Add(1)
			case http.MethodPatch:
				patches.Add(1)
			case http.MethodGet:
				reads.Add(1)
			}
		}

		base.ServeHTTP(w, r)
	}))
	runCard(t, setup)

	first := cardBinding(t, store, id).StatusMessage
	if first.ID == "" || posts.Load() != 1 {
		t.Fatal(first, posts.Load())
	}

	for range 3 {
		makeCardDue(t, db, id, false)
		runCard(t, setup)
	}

	if posts.Load() != 1 || patches.Load() != 0 || reads.Load() != 0 {
		t.Fatal("unchanged card contacted Discord", posts.Load(), patches.Load(), reads.Load())
	}

	source.card.Fields[0].Value = "4"
	source.card.Fields[0].Value = "5"

	makeCardDue(t, db, id, false)
	runCard(t, setup)

	if patches.Load() != 1 || cardBinding(t, store, id).StatusMessage != first {
		t.Fatal("changed card was duplicated instead of edited")
	}

	request := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/channels/400/messages/"+first.ID, nil)
	request.Header.Set("Authorization", "Bot "+fixture.DiscordToken)
	base.ServeHTTP(httptest.NewRecorder(), request)
	makeCardDue(t, db, id, true)
	runCard(t, setup)

	if cardBinding(t, store, id).StatusMessage.ID != "" || posts.Load() != 1 {
		t.Fatal("deletion was not confirmed before replacement")
	}

	makeCardDue(t, db, id, false)
	runCard(t, setup)

	if next := cardBinding(t, store, id).StatusMessage; next.ID == "" || next.ID == first.ID || posts.Load() != 2 {
		t.Fatal("deleted card was not recreated", next, posts.Load())
	}
}

func TestCardsSkipEditsWhenOnlyTruncatedTextChanges(t *testing.T) {
	base := fixture.DiscordHandler()

	var edits atomic.Int64

	db, setup, store, id, source := cardService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			edits.Add(1)
		}

		base.ServeHTTP(w, r)
	}))
	binding := cardBinding(t, store, id)

	binding.Card.Fields = []string{"field.details"}
	if err := store.UpdateBinding(t.Context(), binding); err != nil {
		t.Fatal(err)
	}

	source.card.Fields = []core.CardField{{ID: "field.details", Name: "Details", Value: strings.Repeat("*", 1024), Available: true}}

	runCard(t, setup)
	ref := cardBinding(t, store, id).StatusMessage
	source.card.Fields[0].Value = strings.Repeat("*", 1023) + "a"

	makeCardDue(t, db, id, false)
	runCard(t, setup)

	if edits.Load() != 0 {
		t.Fatal("unchanged Discord content was edited", edits.Load())
	}

	source.card.Fields[0].Value = "a" + strings.Repeat("*", 1023)

	makeCardDue(t, db, id, false)
	runCard(t, setup)

	if edits.Load() != 1 || cardBinding(t, store, id).StatusMessage != ref {
		t.Fatal("visible content change did not edit the saved card")
	}
}

func TestCardsNeverReplaceOnPermissionChannelOrProviderErrors(t *testing.T) {
	for _, response := range []struct {
		name   string
		status int
		body   string
	}{
		{"permission", http.StatusForbidden, `{"message":"private-token-body"}`},
		{"missing channel", http.StatusNotFound, `{"code":10003,"message":"private-token-body"}`},
		{"provider error", http.StatusInternalServerError, `{"message":"private-token-body"}`},
	} {
		t.Run(response.name, func(t *testing.T) {
			base := fixture.DiscordHandler()

			var failing atomic.Bool

			var posts atomic.Int64

			db, setup, store, id, _ := cardService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					posts.Add(1)
				}

				if failing.Load() && r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/messages/") {
					w.WriteHeader(response.status)
					_, _ = w.Write([]byte(response.body))

					return
				}

				base.ServeHTTP(w, r)
			}))
			runCard(t, setup)
			ref := cardBinding(t, store, id).StatusMessage

			failing.Store(true)
			makeCardDue(t, db, id, true)
			runCard(t, setup)
			runCard(t, setup)

			if cardBinding(t, store, id).StatusMessage != ref || posts.Load() != 1 {
				t.Fatal("provider failure caused a replacement")
			}

			status, err := setup.singleServerSettings(t.Context(), "first", accessperm.All())
			if err != nil || status.CardStatus.Error == "" || strings.Contains(status.CardStatus.Error, "private-token-body") {
				t.Fatal(status, err)
			}
		})
	}
}

func TestCardsRecoverLostCreationReceiptsWithoutAdditionalPosts(t *testing.T) {
	base := fixture.DiscordHandler()

	var posts atomic.Int64

	db, setup, store, id, _ := cardService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts.Add(1)
			base.ServeHTTP(httptest.NewRecorder(), r)
			w.WriteHeader(http.StatusInternalServerError)

			return
		}

		base.ServeHTTP(w, r)
	}))
	runCard(t, setup)

	if cardBinding(t, store, id).StatusMessage.ID != "" || posts.Load() != 2 {
		t.Fatal("lost response was treated as success")
	}

	makeCardDue(t, db, id, false)
	runCard(t, setup)

	if cardBinding(t, store, id).StatusMessage.ID == "" || posts.Load() != 2 {
		t.Fatal("receipt recovery created another card")
	}

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/channels/400/messages", nil)
	request.Header.Set("Authorization", "Bot "+fixture.DiscordToken)

	response := httptest.NewRecorder()
	base.ServeHTTP(response, request)

	var messages []json.RawMessage
	if json.Unmarshal(response.Body.Bytes(), &messages) != nil || len(messages) != 1 {
		t.Fatal("nonce retries duplicated card", response.Body)
	}
}

func TestUnconfirmedCardRetriesAndChannelChangesOnlySearchForReceipts(t *testing.T) {
	base := fixture.DiscordHandler()

	var posts atomic.Int64

	db, setup, store, id, _ := cardService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts.Add(1)
			w.WriteHeader(http.StatusInternalServerError)

			return
		}

		base.ServeHTTP(w, r)
	}))
	runCard(t, setup)
	makeCardDue(t, db, id, false)
	runCard(t, setup)

	status, err := setup.singleServerSettings(t.Context(), "first", accessperm.All())
	if err != nil || status.CardStatus.State != "uncertain" || !status.CardStatus.CanRetry {
		t.Fatal(status, err)
	}

	if err := setup.retrySingleServerCard(t.Context(), "first", accessperm.All()); err != nil {
		t.Fatal(err)
	}

	runCard(t, setup)
	binding := cardBinding(t, store, id)

	binding.DestinationID = "402"
	if err := store.UpdateBinding(t.Context(), binding); err != nil {
		t.Fatal(err)
	}

	runCard(t, setup)

	if posts.Load() != 2 || cardBinding(t, store, id).StatusMessage.ID != "" {
		t.Fatal("uncertainty caused a duplicate or moved creation", posts.Load())
	}
}

func TestCardsWaitForOldChannelCleanupAndDeleteWhenDisabled(t *testing.T) {
	base := fixture.DiscordHandler()

	var posts, deletes atomic.Int64

	db, setup, store, id, _ := cardService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts.Add(1)
		}

		if r.Method == http.MethodDelete && deletes.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		base.ServeHTTP(w, r)
	}))
	runCard(t, setup)
	binding := cardBinding(t, store, id)
	old := binding.StatusMessage

	binding.DestinationID = "402"
	if err := store.UpdateBinding(t.Context(), binding); err != nil {
		t.Fatal(err)
	}

	runCard(t, setup)

	if posts.Load() != 1 || cardBinding(t, store, id).StatusMessage != old {
		t.Fatal("new card was created before cleanup")
	}

	makeCardDue(t, db, id, false)
	runCard(t, setup)

	if cardBinding(t, store, id).StatusMessage.ID != "" {
		t.Fatal("old card not cleared after successful deletion")
	}

	makeCardDue(t, db, id, false)
	runCard(t, setup)

	binding = cardBinding(t, store, id)
	if binding.StatusMessage.DestinationID != "402" || posts.Load() != 2 {
		t.Fatal(binding, posts.Load())
	}

	binding.Card.Enabled = false
	if err := store.UpdateBinding(t.Context(), binding); err != nil {
		t.Fatal(err)
	}

	runCard(t, setup)

	if cardBinding(t, store, id).StatusMessage.ID != "" || posts.Load() != 2 {
		t.Fatal("disabled card was not removed")
	}
}

func TestCardCreatedDuringDisableRemainsOwnedForCleanup(t *testing.T) {
	base := fixture.DiscordHandler()

	var disable func()

	db, setup, store, id, _ := cardService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && disable != nil {
			disable()
		}

		base.ServeHTTP(w, r)
	}))
	disable = func() {
		binding := cardBinding(t, store, id)

		binding.Enabled = false
		if err := store.UpdateBinding(t.Context(), binding); err != nil {
			t.Error(err)
		}
	}

	runCard(t, setup)

	if cardBinding(t, store, id).StatusMessage.ID == "" {
		t.Fatal("in-flight receipt was discarded after disable")
	}

	makeCardDue(t, db, id, false)
	runCard(t, setup)

	if cardBinding(t, store, id).StatusMessage.ID != "" {
		t.Fatal("in-flight card was not cleaned up")
	}
}

func TestReenableDuringCleanupDoesNotLoseTheNextCardCreation(t *testing.T) {
	base := fixture.DiscordHandler()

	var reenable func()

	var posts atomic.Int64

	_, setup, store, id, _ := cardService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts.Add(1)
		}

		if r.Method == http.MethodDelete && reenable != nil {
			reenable()
		}

		base.ServeHTTP(w, r)
	}))
	runCard(t, setup)

	binding := cardBinding(t, store, id)

	binding.Card.Enabled = false
	if err := store.UpdateBinding(t.Context(), binding); err != nil {
		t.Fatal(err)
	}

	reenable = func() {
		current, err := store.GetBinding(t.Context(), id)
		if err != nil {
			t.Error(err)
			return
		}

		current.Card.Enabled = true
		if err := store.UpdateBinding(t.Context(), current); err != nil {
			t.Error(err)
		}
	}

	runCard(t, setup)
	runCard(t, setup)

	if posts.Load() != 2 || cardBinding(t, store, id).StatusMessage.ID == "" {
		t.Fatal("re-enable was lost while cleanup was in flight")
	}
}
