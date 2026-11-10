package integrations

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	fixture "campfire/internal/campfire/devfixture/integrations"
	core "campfire/internal/campfire/integrations"
	"campfire/internal/campfire/service/integrations/discord"
	"campfire/internal/shared/model"
)

func TestDiscordFailureKeepsCancellationRetryableAndMissingCredentialsBlocked(t *testing.T) {
	for _, err := range []error{context.Canceled, context.DeadlineExceeded, errors.New("private implementation details")} {
		var failure *core.DeliveryFailure
		if !errors.As(discordDeliveryFailure(err), &failure) || failure.Blocked || failure.Global || failure.Permanent || strings.Contains(failure.Message, "private implementation details") {
			t.Fatal("transient failure became a credential pause or exposed its cause", failure)
		}
	}

	var failure *core.DeliveryFailure
	if !errors.As(discordDeliveryFailure(ErrSecretUnavailable), &failure) || !failure.Blocked || !failure.Global || !failure.Permanent {
		t.Fatal("missing saved credentials did not pause delivery", failure)
	}
}

func TestCanceledSharedPermissionCheckDoesNotBlockLaterNotifications(t *testing.T) {
	db, setup, store, bindingID := deliveryService(t, fixture.DiscordHandler())
	messageID := queueNotification(t, db, store, bindingID, "canceled-access-check")

	claim, err := store.ClaimMessage(t.Context(), time.Now())
	if err != nil || claim == nil {
		t.Fatal(claim, err)
	}

	entered, release := make(chan struct{}, 1), make(chan struct{})

	var once sync.Once

	unblock := func() { once.Do(func() { close(release) }) }
	base := fixture.DiscordHandler()

	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/users/@me" {
			entered <- struct{}{}

			<-release
		}

		base.ServeHTTP(w, r)
	}))
	defer remote.Close()
	defer unblock()

	provider, ok := setup.delivery.Providers["discord"].(*discordProvider)
	if !ok {
		t.Fatal("missing Discord provider")
	}

	provider.client = discord.NewClient(remote.URL, remote.Client())

	checked := make(chan error, 1)
	go func() { checked <- provider.Check(t.Context(), *claim) }()

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("permission check did not start")
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := setup.delivery.deliver(ctx, *claim); err != nil {
		t.Fatal(err)
	}

	unblock()

	if err := <-checked; err != nil {
		t.Fatal(err)
	}

	saved, err := readIntegrationMessage(t.Context(), db, messageID)
	if err != nil || saved.State != model.DeliveryPending || saved.NextAttemptAt <= time.Now().Unix() {
		t.Fatal("canceled notification was not scheduled for retry", saved, err)
	}

	queueNotification(t, db, store, bindingID, "queued-after-restart")

	next, err := store.ClaimMessage(t.Context(), time.Now().Add(time.Hour))
	if err != nil || next == nil {
		t.Fatal("canceled permission wait blocked the connection", next, err)
	}
}
