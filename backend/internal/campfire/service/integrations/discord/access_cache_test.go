package discord

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDeliveryAccessCacheCoalescesExpiresAndInvalidates(t *testing.T) {
	var gets atomic.Int64

	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusForbidden)
			return
		}

		gets.Add(1)
		w.Header().Set("Content-Type", "application/json")

		responses := map[string]string{
			"/users/@me":              `{"id":"100","username":"Bot","bot":true}`,
			"/users/@me/guilds":       `[{"id":"200","name":"Community"}]`,
			"/guilds/200/roles":       `[{"id":"200","permissions":"84992"}]`,
			"/guilds/200/members/100": `{"roles":[]}`,
			"/guilds/200/channels":    `[{"id":"400","guild_id":"200","name":"updates","type":0}]`,
		}
		_, _ = w.Write([]byte(responses[r.URL.Path]))
	}))
	defer remote.Close()

	client := NewClient(remote.URL, remote.Client())

	var workers sync.WaitGroup
	for range 20 {
		workers.Go(func() {
			access, err := client.CheckDelivery(t.Context(), "token", "200")
			if err != nil || len(access.Channels) != 1 || !access.Channels[0].Available {
				t.Error(access, err)
			}
		})
	}

	workers.Wait()

	if gets.Load() != 5 {
		t.Fatal("concurrent delivery repeated checks", gets.Load())
	}

	first, err := client.CheckDelivery(t.Context(), "token", "200")
	if err != nil {
		t.Fatal(err)
	}

	first.Channels[0].Name = "changed"

	second, err := client.CheckDelivery(t.Context(), "token", "200")
	if err != nil || second.Channels[0].Name != "updates" {
		t.Fatal("cached access was mutable", second, err)
	}

	key := accessKey("token", "200")

	client.access.mu.Lock()
	entry := client.access.entries[key]
	entry.expires = time.Now().Add(-time.Second)
	client.access.entries[key] = entry
	client.access.mu.Unlock()

	if _, err := client.CheckDelivery(t.Context(), "token", "200"); err != nil {
		t.Fatal(err)
	}

	if gets.Load() != 10 {
		t.Fatal("expired snapshot reused", gets.Load())
	}

	if _, err := client.Check(t.Context(), "token", "200"); err != nil {
		t.Fatal(err)
	}

	if gets.Load() != 15 {
		t.Fatal("explicit setup check was cached", gets.Load())
	}

	if err := client.do(t.Context(), "token", http.MethodDelete, "/channels/400/messages/500", nil, nil); err == nil {
		t.Fatal("permission failure accepted")
	}

	if _, err := client.CheckDelivery(t.Context(), "token", "200"); err != nil {
		t.Fatal(err)
	}

	if gets.Load() != 20 {
		t.Fatal("authorization failure did not invalidate snapshot", gets.Load())
	}
}

func TestDeliveryAccessWaitHonorsCancellation(t *testing.T) {
	client := NewClient("", nil)
	flight := &accessFlight{done: make(chan struct{})}
	client.access.flights = map[string]*accessFlight{accessKey("token", "200"): flight}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := client.CheckDelivery(ctx, "token", "200"); !errors.Is(err, context.Canceled) {
		t.Fatal("wait ignored cancellation", err)
	}
}
