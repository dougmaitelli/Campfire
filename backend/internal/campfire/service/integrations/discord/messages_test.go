package discord

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf16"

	core "campfire/internal/campfire/integrations"
)

func notificationEvent() core.Event {
	return core.Event{ID: "game:one", ServerID: "server", Category: core.GameEvents, Type: "player.joined", Title: "@everyone **joined**", OccurredAt: time.Now().Unix(), Data: map[string]any{"player": "Alex"}}
}

func TestNotificationSendUsesStableNonceSafeMentionsAndBoundedEmbeds(t *testing.T) {
	var calls atomic.Int64

	nonce := MessageNonce("binding", "event")

	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)

		var payload struct {
			Nonce    string `json:"nonce"`
			Enforce  bool   `json:"enforce_nonce"`
			Mentions struct {
				Parse []string `json:"parse"`
			} `json:"allowed_mentions"`
			Embeds []messageEmbed `json:"embeds"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}

		if payload.Nonce != nonce || !payload.Enforce || payload.Mentions.Parse == nil || len(payload.Mentions.Parse) != 0 {
			t.Error("missing nonce or mention protection", payload)
		}

		embed := payload.Embeds[0]
		if len(utf16.Encode([]rune(embed.Title))) > 256 || len(utf16.Encode([]rune(embed.Description))) > 2000 || len(embed.Fields) > 10 || embed.Footer.Text != "Campfire notification · "+nonce {
			t.Error("invalid Discord limits", embed)
		}

		if calls.Load() == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		_, _ = w.Write([]byte(`{"id":"500","channel_id":"400"}`))
	}))
	defer remote.Close()

	event := notificationEvent()
	event.Body = strings.Repeat("😀", 2000)

	id, err := NewClient(remote.URL, remote.Client()).SendEvent(t.Context(), "private", "400", strings.Repeat("Server", 200), nonce, event)
	if err != nil || id != "500" || calls.Load() != 2 || len(nonce) > 25 || nonce != MessageNonce("binding", "event") {
		t.Fatal(id, err, calls.Load())
	}
}

func TestDiscordRateLimitsHonorFractionalGlobalAndChannelScopes(t *testing.T) {
	var calls atomic.Int64

	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "2.25")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"retry_after":3.75,"global":true,"message":"private body"}`))
	}))
	defer remote.Close()

	client := NewClient(remote.URL, remote.Client())
	_, err := client.SendEvent(t.Context(), "token", "400", "Server", "nonce", notificationEvent())

	var failure *Failure
	if !errors.As(err, &failure) || !failure.Global || failure.RetryAfter < 3750*time.Millisecond || failure.RetryAfter > 4*time.Second || strings.Contains(err.Error(), "private body") {
		t.Fatal(err, failure)
	}

	_, err = client.SendEvent(t.Context(), "token", "401", "Server", "nonce", notificationEvent())
	if !errors.As(err, &failure) || !failure.Global || calls.Load() != 1 {
		t.Fatal("global cooldown was ignored", err, calls.Load())
	}

	owner, route, _ := rateKeys("token", "POST", "/channels/400/messages")
	if err := client.rates.check(owner, route, time.Now().Add(5*time.Second)); err != nil {
		t.Fatal("cooldown did not expire", err)
	}
}

func TestDiscordRecoversOnlyItsOwnMessageReference(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Error("reconciliation attempted a mutation")
		}

		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": "501", "channel_id": "400", "author": map[string]string{"id": "999"}, "embeds": []map[string]any{{"footer": map[string]string{"text": "Campfire notification · nonce"}}}},
			{"id": "500", "channel_id": "400", "author": map[string]string{"id": "100"}, "embeds": []map[string]any{{"footer": map[string]string{"text": "Campfire notification · nonce"}}}},
		})
	}))
	defer remote.Close()

	id, err := NewClient(remote.URL, remote.Client()).FindEvent(context.Background(), "private", "400", "100", "nonce", time.Now().Unix())
	if err != nil || id != "500" {
		t.Fatal(id, err)
	}
}
