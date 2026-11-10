package integrations

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	fixture "campfire/internal/campfire/devfixture/integrations"
	core "campfire/internal/campfire/integrations"
	"campfire/internal/shared/model"
	accessperm "campfire/internal/shared/permissions"
)

type fakeMessageProvider struct {
	check func(context.Context, repository.IntegrationClaim) error
	send  func(context.Context, repository.IntegrationClaim) (string, error)
}

func (p *fakeMessageProvider) Check(ctx context.Context, claim repository.IntegrationClaim) error {
	if p.check != nil {
		return p.check(ctx, claim)
	}

	return nil
}
func (p *fakeMessageProvider) Send(ctx context.Context, claim repository.IntegrationClaim) (string, error) {
	if p.send != nil {
		return p.send(ctx, claim)
	}

	return "receipt", nil
}
func (*fakeMessageProvider) Find(context.Context, repository.IntegrationClaim) (string, error) {
	return "receipt", nil
}

func deliveryService(t *testing.T, remote http.Handler) (*database.Database, *DiscordIntegrationService, *repository.IntegrationRepository, string) {
	t.Helper()
	db, _, setup := discordService(t, remote)
	seedIntegrationServers(t, db)

	if _, err := setup.Save(t.Context(), testPrincipal(true), DiscordIntegrationInput{Enabled: true, GuildID: "200", Token: fixture.DiscordToken}); err != nil {
		t.Fatal(err)
	}

	if _, err := setup.saveSingleServerSettings(t.Context(), "first", accessperm.All(), ServerIntegrationInput{Enabled: true, ChannelID: "400", Subscriptions: []core.Subscription{{Category: core.GameEvents}}}); err != nil {
		t.Fatal(err)
	}

	store := repository.NewIntegrationRepository(db.ORM())

	bindings, err := store.ListBindings(t.Context(), "first")
	if err != nil || len(bindings) != 1 {
		t.Fatal(bindings, err)
	}

	return db, setup, store, bindings[0].ID
}

func queueNotification(t *testing.T, db *database.Database, store *repository.IntegrationRepository, bindingID, id string) int64 {
	t.Helper()

	binding, err := store.GetBinding(t.Context(), bindingID)
	if err != nil {
		t.Fatal(err)
	}

	event := core.Event{ID: "game:" + id, ServerID: binding.ServerID, Category: core.GameEvents, Type: "player.joined", Title: "Player joined", OccurredAt: time.Now().Unix(), Data: map[string]any{"player": "Alex"}}
	if err := store.RouteEvent(t.Context(), event, time.Now()); err != nil {
		t.Fatal(err)
	}

	var message database.IntegrationMessage
	if err := db.ORM().Where("binding_id = ? AND event_id = ?", bindingID, event.ID).Take(&message).Error; err != nil {
		t.Fatal(err)
	}

	return message.ID
}

func TestDeliveryWorkerRecoversLostReceiptsWithoutDuplicatePosts(t *testing.T) {
	base := fixture.DiscordHandler()

	var posts atomic.Int64

	db, setup, store, binding := deliveryService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts.Add(1)
			base.ServeHTTP(httptest.NewRecorder(), r)
			w.WriteHeader(http.StatusInternalServerError)

			return
		}

		base.ServeHTTP(w, r)
	}))

	id := queueNotification(t, db, store, binding, "lost-receipt")
	if err := setup.delivery.RunOnce(t.Context()); err != nil {
		t.Fatal(err)
	}

	message, err := readIntegrationMessage(t.Context(), db, id)
	if err != nil || message.State != core.MessageUncertain || posts.Load() != 2 {
		t.Fatal(message, err, posts.Load())
	}

	if err := db.ORM().Model(&database.IntegrationMessage{}).Where("id = ?", id).Update("next_attempt_at", time.Now().Unix()-1).Error; err != nil {
		t.Fatal(err)
	}

	if err := setup.delivery.RunOnce(t.Context()); err != nil {
		t.Fatal(err)
	}

	message, err = readIntegrationMessage(t.Context(), db, id)
	if err != nil || message.State != model.DeliveryDelivered || message.ExternalMessageID == "" || posts.Load() != 2 {
		t.Fatal("recovery reposted or failed", message, err, posts.Load())
	}
}

func TestUnconfirmedDeliveryPausesAndExplicitCheckNeverReposts(t *testing.T) {
	base := fixture.DiscordHandler()

	var posts atomic.Int64

	db, setup, store, binding := deliveryService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts.Add(1)
			w.WriteHeader(http.StatusInternalServerError)

			return
		}

		base.ServeHTTP(w, r)
	}))

	id := queueNotification(t, db, store, binding, "unknown")
	if err := setup.delivery.RunOnce(t.Context()); err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Model(&database.IntegrationMessage{}).Where("id = ?", id).Update("next_attempt_at", time.Now().Unix()-1).Error; err != nil {
		t.Fatal(err)
	}

	if err := setup.delivery.RunOnce(t.Context()); err != nil {
		t.Fatal(err)
	}

	message, err := readIntegrationMessage(t.Context(), db, id)
	if err != nil || message.State != core.MessageUncertain || message.NextAttemptAt != 0 || posts.Load() != 2 {
		t.Fatal(message, err, posts.Load())
	}

	if err := setup.retrySingleServerMessage(t.Context(), "first", accessperm.All(), id); err != nil {
		t.Fatal(err)
	}

	if err := setup.delivery.RunOnce(t.Context()); err != nil {
		t.Fatal(err)
	}

	if posts.Load() != 2 {
		t.Fatal("explicit check created another notification")
	}
}

func TestDeliveryRetriesAreBoundedSanitizedAndExplicitlyRecoverable(t *testing.T) {
	db, setup, store, binding := deliveryService(t, fixture.DiscordHandler())
	provider := &fakeMessageProvider{check: func(context.Context, repository.IntegrationClaim) error {
		return errors.New("private-token-must-not-escape")
	}}
	setup.delivery.Providers["discord"] = provider

	id := queueNotification(t, db, store, binding, "retry")
	if err := setup.delivery.RunOnce(t.Context()); err != nil {
		t.Fatal(err)
	}

	message, err := readIntegrationMessage(t.Context(), db, id)
	if err != nil || message.State != model.DeliveryPending || message.NextAttemptAt <= time.Now().Unix() || strings.Contains(message.Error, "private-token") {
		t.Fatal(message, err)
	}

	if err := db.ORM().Model(&database.IntegrationMessage{}).Where("id = ?", id).Updates(map[string]any{"attempts": core.DeliveryAttempts - 1, "next_attempt_at": time.Now().Unix() - 1}).Error; err != nil {
		t.Fatal(err)
	}

	if err := setup.delivery.RunOnce(t.Context()); err != nil {
		t.Fatal(err)
	}

	message, err = readIntegrationMessage(t.Context(), db, id)
	if err != nil || message.State != model.DeliveryFailed || message.NextAttemptAt != 0 {
		t.Fatal(message, err)
	}

	provider.check = nil

	if err := setup.retrySingleServerMessage(t.Context(), "first", accessperm.All(), id); err != nil {
		t.Fatal(err)
	}

	if err := setup.delivery.RunOnce(t.Context()); err != nil {
		t.Fatal(err)
	}

	message, err = readIntegrationMessage(t.Context(), db, id)
	if err != nil || message.State != model.DeliveryDelivered || message.Attempts != 1 {
		t.Fatal(message, err)
	}
}

func TestSlowDestinationDoesNotBlockOtherDestinations(t *testing.T) {
	db, setup, store, first := deliveryService(t, fixture.DiscordHandler())

	binding, err := store.GetBinding(t.Context(), first)
	if err != nil {
		t.Fatal(err)
	}

	binding.ID, binding.ServerID, binding.DestinationID = "second-binding", "second", "402"
	if err := store.CreateBinding(t.Context(), binding); err != nil {
		t.Fatal(err)
	}

	entered, delivered := make(chan struct{}, 1), make(chan struct{}, 3)
	setup.delivery.Providers["discord"] = &fakeMessageProvider{
		check: func(ctx context.Context, claim repository.IntegrationClaim) error {
			if claim.DestinationID == "400" {
				entered <- struct{}{}

				<-ctx.Done()

				return ctx.Err()
			}

			return nil
		},
		send: func(context.Context, repository.IntegrationClaim) (string, error) {
			delivered <- struct{}{}
			return "receipt", nil
		},
	}

	queueNotification(t, db, store, first, "slow")

	setup.Start(t.Context())
	defer setup.Close()

	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("slow destination was not started")
	}

	for _, id := range []string{"one", "two", "three"} {
		queueNotification(t, db, store, binding.ID, id)
	}

	for range 3 {
		select {
		case <-delivered:
		case <-time.After(3 * time.Second):
			t.Fatal("slow destination blocked another destination")
		}
	}
}

func TestPermanentDeliveryFailuresPauseOnlyTheirScopeUntilExplicitRetry(t *testing.T) {
	for _, global := range []bool{false, true} {
		t.Run(fmt.Sprintf("global_%t", global), func(t *testing.T) {
			db, setup, store, first := deliveryService(t, fixture.DiscordHandler())

			binding, err := store.GetBinding(t.Context(), first)
			if err != nil {
				t.Fatal(err)
			}

			binding.ID, binding.ServerID, binding.DestinationID = "second-binding", "second", "402"
			if err := store.CreateBinding(t.Context(), binding); err != nil {
				t.Fatal(err)
			}

			var sends atomic.Int64

			provider := &fakeMessageProvider{check: func(_ context.Context, claim repository.IntegrationClaim) error {
				if global || claim.DestinationID == "400" {
					return &core.DeliveryFailure{Message: "Connection needs attention", Permanent: true, Blocked: true, Global: global}
				}

				return nil
			}, send: func(context.Context, repository.IntegrationClaim) (string, error) {
				sends.Add(1)
				return "receipt", nil
			}}
			setup.delivery.Providers["discord"] = provider
			id := queueNotification(t, db, store, first, "blocked")
			queueNotification(t, db, store, first, "waiting")
			queueNotification(t, db, store, binding.ID, "other-channel")

			if err := setup.delivery.RunOnce(t.Context()); err != nil {
				t.Fatal(err)
			}

			if err := setup.delivery.RunOnce(t.Context()); err != nil {
				t.Fatal(err)
			}

			expected := int64(1)
			if global {
				expected = 0
			}

			if sends.Load() != expected {
				t.Fatal("permanent failure did not isolate its scope", sends.Load())
			}

			provider.check = nil

			if err := setup.retrySingleServerMessage(t.Context(), "first", accessperm.All(), id); err != nil {
				t.Fatal(err)
			}

			for range 2 {
				if err := setup.delivery.RunOnce(t.Context()); err != nil {
					t.Fatal(err)
				}
			}

			if sends.Load() != expected+2 {
				t.Fatal("explicit retry did not release pending work", sends.Load())
			}
		})
	}
}
