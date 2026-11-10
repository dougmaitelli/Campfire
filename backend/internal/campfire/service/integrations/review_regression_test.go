package integrations

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync/atomic"
	"testing"

	fixture "campfire/internal/campfire/devfixture/integrations"
	"campfire/internal/shared/permissions"
)

func TestDeliveryReusesCheckedDiscordPermissions(t *testing.T) {
	var gets, posts atomic.Int64

	remote := fixture.DiscordHandler()
	db, setup, store, id := deliveryService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			gets.Add(1)
		}

		if r.Method == http.MethodPost {
			posts.Add(1)
		}

		remote.ServeHTTP(w, r)
	}))

	gets.Store(0)
	posts.Store(0)

	for index := range 3 {
		queueNotification(t, db, store, id, "cached-"+strconv.Itoa(index))

		if err := setup.delivery.RunOnce(t.Context()); err != nil {
			t.Fatal(err)
		}
	}

	if gets.Load() != 0 || posts.Load() != 3 {
		t.Fatal("delivery repeated setup checks", gets.Load(), posts.Load())
	}
}

func TestCardCleanupDoesNotRequireSendPermission(t *testing.T) {
	var denySend atomic.Bool

	var deletes atomic.Int64

	remote := fixture.DiscordHandler()
	_, setup, store, id, _ := cardService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deletes.Add(1)
		}

		if denySend.Load() && r.URL.Path == "/guilds/200/channels" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": "400", "guild_id": "200", "type": 0, "name": "readable", "permission_overwrites": []map[string]any{{"id": "200", "type": 0, "allow": "0", "deny": "2048"}}}})

			return
		}

		remote.ServeHTTP(w, r)
	}))
	runCard(t, setup)

	binding := cardBinding(t, store, id)
	if binding.StatusMessage.ID == "" {
		t.Fatal("card was not created")
	}

	denySend.Store(true)

	access, err := setup.client.Check(t.Context(), fixture.DiscordToken, "200")
	if err != nil || len(access.Channels) != 1 || access.Channels[0].Available {
		t.Fatal(access, err)
	}

	if _, err := setup.SaveServerSettings(t.Context(), binding.ServerID, binding.ID, permissions.All(), ServerIntegrationInput{
		Enabled: true, ChannelID: binding.DestinationID,
	}); err != nil {
		t.Fatal(err)
	}

	runCard(t, setup)

	if deletes.Load() != 1 || cardBinding(t, store, id).StatusMessage.ID != "" {
		t.Fatal("cleanup required publishing permission", deletes.Load())
	}
}
