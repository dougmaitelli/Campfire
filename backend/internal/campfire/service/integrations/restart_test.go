package integrations

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	fixture "campfire/internal/campfire/devfixture/integrations"
	core "campfire/internal/campfire/integrations"
	"campfire/internal/campfire/service/integrations/discord"
	accessperm "campfire/internal/shared/permissions"
)

func TestIntegrationRestartRecoversReceiptsCredentialsAndPendingCleanup(t *testing.T) {
	base := fixture.DiscordHandler()

	var loseCreates, loseDeletes atomic.Bool

	var cardPosts, notificationPosts atomic.Int64

	loseCreates.Store(true)

	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/messages") {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusBadRequest)

				return
			}

			r.Body = io.NopCloser(bytes.NewReader(body))

			var payload struct {
				Embeds []struct {
					Footer struct {
						Text string `json:"text"`
					} `json:"footer"`
				} `json:"embeds"`
			}
			if err := json.Unmarshal(body, &payload); err != nil || len(payload.Embeds) == 0 {
				t.Error("missing embed", err)
				w.WriteHeader(http.StatusBadRequest)

				return
			}

			if strings.HasPrefix(payload.Embeds[0].Footer.Text, "Campfire server · ") {
				cardPosts.Add(1)
			} else {
				notificationPosts.Add(1)
			}

			if loseCreates.Load() {
				base.ServeHTTP(httptest.NewRecorder(), r)
				w.WriteHeader(http.StatusInternalServerError)

				return
			}
		}

		if r.Method == http.MethodDelete && loseDeletes.Load() {
			base.ServeHTTP(httptest.NewRecorder(), r)
			w.WriteHeader(http.StatusInternalServerError)

			return
		}

		base.ServeHTTP(w, r)
	}))
	defer remote.Close()

	root := t.TempDir()
	source := &staticCardSource{card: core.ServerCard{Name: "First", State: "running", Fields: []core.CardField{{ID: "players", Name: "Players", Value: "3", Available: true}}}}
	open := func() (*database.Database, *DiscordIntegrationService, *repository.IntegrationRepository) {
		db, err := database.Open(t.Context(), filepath.Join(root, "panel.db"))
		if err != nil {
			t.Fatal(err)
		}

		t.Cleanup(func() { _ = db.Close() })

		setup := NewDiscordIntegrationService(db.ORM(), Secrets{Directory: filepath.Join(root, "integration-secrets")}, discord.NewClient(remote.URL, remote.Client())).WithCards(source)

		return db, setup, repository.NewIntegrationRepository(db.ORM())
	}
	db, setup, store := open()
	seedIntegrationServers(t, db)

	if _, err := setup.Save(t.Context(), testPrincipal(true), DiscordIntegrationInput{Enabled: true, GuildID: "200", Token: fixture.DiscordToken}); err != nil {
		t.Fatal(err)
	}

	if _, err := setup.saveSingleServerSettings(t.Context(), "first", accessperm.All(), ServerIntegrationInput{Enabled: true, ChannelID: "400", Subscriptions: []core.Subscription{{Category: core.GameEvents}}, Card: core.CardSettings{Enabled: true, Fields: []string{"players"}}}); err != nil {
		t.Fatal(err)
	}

	bindings, err := store.ListBindings(t.Context(), "first")
	if err != nil || len(bindings) != 1 {
		t.Fatal(bindings, err)
	}

	id := bindings[0].ID
	queueNotification(t, db, store, id, "before-restart")

	if err := setup.delivery.RunOnce(t.Context()); err != nil {
		t.Fatal(err)
	}

	runCard(t, setup)
	queueNotification(t, db, store, id, "pending-across-restart")

	if cardPosts.Load() != 2 || notificationPosts.Load() != 2 {
		t.Fatal("lost replies were not attempted with stable nonces")
	}

	setup.Close()

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	loseCreates.Store(false)

	db, setup, store = open()

	status, err := setup.Status(t.Context(), testPrincipal(true))
	if err != nil || !status.Healthy {
		t.Fatal("saved encrypted credentials could not be reopened", status, err)
	}

	if err := db.ORM().Model(&database.IntegrationMessage{}).Where("binding_id = ? AND category = ? AND state = ?", id, core.GameEvents, core.MessageUncertain).Update("next_attempt_at", 1).Error; err != nil {
		t.Fatal(err)
	}

	makeCardDue(t, db, id, false)
	queueNotification(t, db, store, id, "before-restart")

	for range 2 {
		if err := setup.delivery.RunOnce(t.Context()); err != nil {
			t.Fatal(err)
		}
	}

	runCard(t, setup)

	ref := cardBinding(t, store, id).StatusMessage
	if ref.ID == "" || cardPosts.Load() != 2 || notificationPosts.Load() != 3 {
		t.Fatal("restart recovery duplicated accepted messages", ref, cardPosts.Load(), notificationPosts.Load())
	}

	items, err := setup.singleServerMessages(t.Context(), "first", accessperm.All(), 0, "")
	if err != nil || len(items) != 2 || items[0].State != "delivered" || items[1].State != "delivered" {
		t.Fatal("pending notification did not survive restart", items, err)
	}

	binding := cardBinding(t, store, id)

	binding.Card.Enabled = false
	if err := store.UpdateBinding(t.Context(), binding); err != nil {
		t.Fatal(err)
	}

	loseDeletes.Store(true)
	runCard(t, setup)

	if cardBinding(t, store, id).StatusMessage != ref {
		t.Fatal("lost cleanup reply discarded message ownership")
	}

	setup.Close()

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	loseDeletes.Store(false)

	db, setup, store = open()
	defer setup.Close()

	makeCardDue(t, db, id, false)
	runCard(t, setup)

	settings, err := setup.singleServerSettings(t.Context(), "first", accessperm.All())
	if err != nil || settings.CardStatus.State != "disabled" || cardBinding(t, store, id).StatusMessage.ID != "" || cardPosts.Load() != 2 {
		t.Fatal("restart did not complete idempotent cleanup", settings, err)
	}

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/channels/400/messages", nil)
	request.Header.Set("Authorization", "Bot "+fixture.DiscordToken)

	response := httptest.NewRecorder()
	base.ServeHTTP(response, request)

	var messages []json.RawMessage
	if json.Unmarshal(response.Body.Bytes(), &messages) != nil || len(messages) != 2 {
		t.Fatal("unexpected provider messages after recovery and cleanup", response.Body)
	}
}
