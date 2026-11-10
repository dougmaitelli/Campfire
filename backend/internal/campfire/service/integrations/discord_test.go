package integrations

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	fixture "campfire/internal/campfire/devfixture/integrations"
	"campfire/internal/campfire/service/integrations/discord"
)

type testPrincipal bool

func (p testPrincipal) CanManageIntegrations() bool { return bool(p) }

func discordService(t *testing.T, remote http.Handler) (*database.Database, Secrets, *DiscordIntegrationService) {
	t.Helper()

	db, err := database.Open(t.Context(), filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	server := httptest.NewServer(remote)
	t.Cleanup(server.Close)
	secrets := Secrets{Directory: filepath.Join(t.TempDir(), "secrets")}

	return db, secrets, NewDiscordIntegrationService(db.ORM(), secrets, discord.NewClient(server.URL, server.Client()))
}

func TestDiscordSetupPersistsPrivateCredentialsAndRotatesThem(t *testing.T) {
	db, secrets, setup := discordService(t, fixture.DiscordHandler())
	ctx := t.Context()
	input := DiscordIntegrationInput{Enabled: true, GuildID: "200", Token: fixture.DiscordToken}

	status, err := setup.Save(ctx, testPrincipal(true), input)
	if err != nil || !status.Configured || !status.Healthy || len(status.Channels) != 3 || status.Channels[1].Available || len(status.Channels[1].Missing) != 1 {
		t.Fatal("setup did not validate effective channel permissions", status, err)
	}

	store := repository.NewIntegrationRepository(db.ORM())

	connections, err := store.ListConnections(ctx)
	if err != nil || len(connections) != 1 {
		t.Fatal(connections, err)
	}

	previous := connections[0]

	encoded, err := json.Marshal(status)
	if err != nil || strings.Contains(string(encoded), input.Token) || strings.Contains(string(encoded), "credential_ref") || strings.Contains(string(previous.Config), input.Token) || previous.CredentialRef == input.Token {
		t.Fatal("setup exposed credential material", err)
	}

	reopened := NewDiscordIntegrationService(db.ORM(), Secrets{Directory: secrets.Directory}, setup.client)
	if restored, err := reopened.Status(ctx, testPrincipal(true)); err != nil || !restored.Healthy {
		t.Fatal("saved configuration could not be reopened", restored, err)
	}

	if err := reopened.Test(ctx, testPrincipal(true), "401"); err == nil {
		t.Fatal("test message was sent without required permissions")
	}

	if err := reopened.Test(ctx, testPrincipal(true), "999"); err == nil {
		t.Fatal("test message was sent outside the selected guild")
	}

	if err := reopened.Test(ctx, testPrincipal(true), "400"); err != nil {
		t.Fatal(err)
	}

	input.Token += "-rotated"
	if _, err := reopened.Save(ctx, testPrincipal(true), input); err != nil {
		t.Fatal(err)
	}

	if _, err := secrets.Get(previous.CredentialRef); !errors.Is(err, ErrSecretUnavailable) {
		t.Fatal("retired token was not removed", err)
	}

	connections, err = store.ListConnections(ctx)
	if err != nil || connections[0].ID != previous.ID || connections[0].CredentialRef == previous.CredentialRef {
		t.Fatal("token rotation duplicated the connection or reused its secret", connections, err)
	}
}

func TestDiscordDisableWorksWhenRemoteAccessFails(t *testing.T) {
	var unavailable atomic.Bool

	remote := fixture.DiscordHandler()
	_, _, setup := discordService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if unavailable.Load() {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		remote.ServeHTTP(w, r)
	}))

	ctx := t.Context()
	if _, err := setup.Save(ctx, testPrincipal(true), DiscordIntegrationInput{Enabled: true, GuildID: "200", Token: fixture.DiscordToken}); err != nil {
		t.Fatal(err)
	}

	unavailable.Store(true)

	if status, err := setup.Status(ctx, testPrincipal(true)); err != nil || status.Healthy || status.Error == "" {
		t.Fatal("revoked token was not reported as unhealthy", status, err)
	}

	if status, err := setup.Save(ctx, testPrincipal(true), DiscordIntegrationInput{GuildID: "200"}); err != nil || status.Enabled {
		t.Fatal("remote failure prevented disabling the integration", status, err)
	}

	if err := setup.Test(ctx, testPrincipal(true), "400"); !errors.Is(err, repository.ErrIntegrationDisabled) {
		t.Fatal("disabled connection could send test messages", err)
	}
}

func TestDiscordCanSaveVerifiedCredentialsWhileDisabled(t *testing.T) {
	_, _, setup := discordService(t, fixture.DiscordHandler())

	status, err := setup.Save(t.Context(), testPrincipal(true), DiscordIntegrationInput{GuildID: "200", Token: fixture.DiscordToken})
	if err != nil || !status.Configured || status.Enabled || status.Healthy || status.Error != "" {
		t.Fatal("disabled setup reported an active integration", status, err)
	}
}

func TestDiscordStaleSaveCannotUndoANewerDisable(t *testing.T) {
	base := fixture.DiscordHandler()

	var delay atomic.Bool

	entered, release := make(chan struct{}), make(chan struct{})

	_, _, setup := discordService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if delay.Load() && r.URL.Path == "/users/@me" {
			close(entered)

			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}

		base.ServeHTTP(w, r)
	}))
	if _, err := setup.Save(t.Context(), testPrincipal(true), DiscordIntegrationInput{Enabled: true, GuildID: "200", Token: fixture.DiscordToken}); err != nil {
		t.Fatal(err)
	}

	delay.Store(true)

	result := make(chan error, 1)

	go func() {
		_, err := setup.Save(t.Context(), testPrincipal(true), DiscordIntegrationInput{Enabled: true, GuildID: "200"})
		result <- err
	}()

	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("save did not reach connection validation")
	}

	if _, err := setup.Save(t.Context(), testPrincipal(true), DiscordIntegrationInput{GuildID: "200"}); err != nil {
		close(release)
		t.Fatal(err)
	}

	close(release)

	if err := <-result; !errors.Is(err, repository.ErrIntegrationConflict) {
		t.Fatal("stale save undid a newer disable", err)
	}

	status, err := setup.Status(t.Context(), testPrincipal(true))
	if err != nil || status.Enabled {
		t.Fatal("global disable was lost", status, err)
	}
}

func TestDiscordAdminBoundaryPrecedesRemoteAndStorageAccess(t *testing.T) {
	var calls atomic.Int64

	db, _, setup := discordService(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	ctx := t.Context()
	denied := testPrincipal(false)
	_, statusErr := setup.Status(ctx, denied)
	_, previewErr := setup.Preview(ctx, denied, fixture.DiscordToken)
	_, saveErr := setup.Save(ctx, denied, DiscordIntegrationInput{Enabled: true, GuildID: "200", Token: fixture.DiscordToken})

	_, channelsErr := setup.Channels(ctx, denied)
	for _, err := range []error{statusErr, previewErr, saveErr, channelsErr, setup.Test(ctx, denied, "400")} {
		if !errors.Is(err, repository.ErrAccessDenied) {
			t.Fatal("service accepted a non-administrator", err)
		}
	}

	var count int64
	if err := db.ORM().Model(&database.Integration{}).Count(&count).Error; err != nil || count != 0 || calls.Load() != 0 {
		t.Fatal("denied setup reached persistence or Discord", count, calls.Load(), err)
	}
}

func TestDiscordConcurrentSetupCreatesOneConnection(t *testing.T) {
	var arrivals atomic.Int64

	ready := make(chan struct{})
	remote := fixture.DiscordHandler()
	db, _, setup := discordService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/users/@me" {
			if arrivals.Add(1) == 2 {
				close(ready)
			}

			select {
			case <-ready:
			case <-r.Context().Done():
				return
			}
		}

		remote.ServeHTTP(w, r)
	}))
	results := make(chan error, 2)

	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			_, err := setup.Save(t.Context(), testPrincipal(true), DiscordIntegrationInput{Enabled: true, GuildID: "200", Token: fixture.DiscordToken})
			results <- err
		})
	}

	wg.Wait()
	close(results)

	succeeded, conflicted := 0, 0

	for err := range results {
		if err == nil {
			succeeded++
		} else if errors.Is(err, repository.ErrIntegrationConflict) {
			conflicted++
		} else {
			t.Fatal(err)
		}
	}

	var count int64
	if err := db.ORM().Model(&database.Integration{}).Count(&count).Error; err != nil || count != 1 || succeeded != 1 || conflicted != 1 {
		t.Fatal("concurrent setup bypassed the singleton policy", count, succeeded, conflicted, err)
	}
}
