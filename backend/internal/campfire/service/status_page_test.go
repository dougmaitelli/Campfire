package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"campfire/internal/campfire/database/repository"
	integrationservice "campfire/internal/campfire/service/integrations"
	"campfire/internal/campfire/service/integrations/discord"
	"campfire/internal/campfire/statuspage"
	fixture "campfire/internal/devfixture/integrations"
	"campfire/internal/shared/model"
)

func TestStatusPageCardEditsSameDiscordMessageAsObservationsChangeAndExpire(t *testing.T) {
	ctx := t.Context()
	activity, db, _, _ := activityFixture(t)
	deployment := activity.deployment
	remote := fixture.DiscordHandler()

	type message struct {
		discord.CardMessage
		ID string `json:"id"`
	}

	changes := make(chan message, 4)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodPatch {
			remote.ServeHTTP(w, r)
			return
		}

		recorder := httptest.NewRecorder()
		remote.ServeHTTP(recorder, r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(recorder.Code)
		_, _ = w.Write(recorder.Body.Bytes())

		var published message
		if err := json.Unmarshal(recorder.Body.Bytes(), &published); err != nil {
			t.Error(err)
			return
		}

		select {
		case changes <- published:
		case <-r.Context().Done():
		}
	}))
	defer server.Close()

	bot := integrationservice.NewDiscordIntegrationService(db.ORM(), integrationservice.Secrets{Directory: t.TempDir()}, discord.NewClient(server.URL, server.Client()))

	admin := Principal{Admin: true}
	if _, err := bot.Save(ctx, admin, integrationservice.DiscordIntegrationInput{Enabled: true, GuildID: "200", Token: fixture.DiscordToken}); err != nil {
		t.Fatal(err)
	}

	pages := &StatusPageService{Store: repository.NewStatusPageRepository(db.ORM()), Deployment: deployment, Discord: bot}
	if _, err := pages.Save(ctx, admin, "", statuspage.Input{Title: "Community", Slug: "community", DiscordChannelID: new("400"), Entries: []statuspage.Selection{{ServerID: "game", Name: "Public realm"}}}); err != nil {
		t.Fatal(err)
	}

	deployment.cacheStatus("node", model.RuntimeStatus{ServerID: "game", State: model.RuntimeStateRunning, Ready: true})

	bot.WithStatusPageCards(pages.StatusPageCard).Start(ctx)
	defer bot.Close()

	expectMessage := func(state string) string {
		t.Helper()

		select {
		case published := <-changes:
			if published.ID == "" || len(published.Embeds) != 1 || len(published.Embeds[0].Fields) != 1 {
				t.Fatal("unexpected Discord message", published)
			}

			field := published.Embeds[0].Fields[0]
			if field.Name != "Public realm" || field.Value != state {
				t.Fatal("unexpected public availability", field, state)
			}

			return published.ID
		case <-time.After(20 * time.Second):
			t.Fatalf("timed out waiting for %s", state)
			return ""
		}
	}
	id := expectMessage("Online")

	deployment.cacheStatus("node", model.RuntimeStatus{ServerID: "game", State: model.RuntimeStateStopped})

	if next := expectMessage("Offline"); next != id {
		t.Fatalf("card changed from %s to %s", id, next)
	}

	deployment.mu.Lock()
	deployment.observed["game"] = time.Now().Add(-runtimeStatusLifetime)
	deployment.mu.Unlock()

	if next := expectMessage("Unknown"); next != id {
		t.Fatalf("expired observation replaced card %s with %s", id, next)
	}
}

func TestPublicAvailabilityDoesNotInventOnlineOrOffline(t *testing.T) {
	for _, tc := range []struct {
		state model.RuntimeState
		ready bool
		want  statuspage.State
	}{
		{model.RuntimeStateRunning, true, statuspage.Online},
		{model.RuntimeStateRunning, false, statuspage.Offline},
		{model.RuntimeStateStopped, false, statuspage.Offline},
		{model.RuntimeStateStarting, false, statuspage.Offline},
		{model.RuntimeStateRestarting, false, statuspage.Offline},
		{model.RuntimeStateNotInstalled, false, statuspage.Offline},
		{model.RuntimeStateUnknown, true, statuspage.Unknown},
		{model.RuntimeStateOffline, true, statuspage.Unknown},
		{model.RuntimeState("future-state"), true, statuspage.Unknown},
	} {
		if got := publicAvailability(model.RuntimeStatus{State: tc.state, Ready: tc.ready}); got != tc.want {
			t.Fatal(tc, got)
		}
	}
}

func TestCachedRuntimeSnapshotsOwnLockingAndKeepTargetOrder(t *testing.T) {
	d := &DeploymentService{}
	d.cacheStatus("node", model.RuntimeStatus{ServerID: "game", State: model.RuntimeStateRunning, Ready: true})
	d.cacheStatus("node", model.RuntimeStatus{ServerID: "stale", State: model.RuntimeStateRunning, Ready: true})
	d.mu.Lock()
	d.observed["stale"] = time.Now().Add(-runtimeStatusLifetime)
	d.mu.Unlock()

	targets := []model.ResourceTarget{{ID: "game", SparkID: "node"}, {ID: "missing", SparkID: "node"}, {ID: "game", SparkID: "other-node"}, {ID: "stale", SparkID: "node"}}

	observations := d.cachedRuntimeStatuses(targets)
	if len(observations) != len(targets) {
		t.Fatal(observations)
	}

	for index, observation := range observations {
		if observation.Status.ServerID != targets[index].ID {
			t.Fatal("target order changed", observations)
		}

		if index == 0 {
			if observation.Status.State != model.RuntimeStateRunning || observation.Observed.IsZero() || observation.ValidFor <= 0 || observation.ValidFor > runtimeStatusLifetime {
				t.Fatal(observation)
			}
		} else if observation.Status.State != model.RuntimeStateUnknown || !observation.Observed.IsZero() || observation.ValidFor != 0 {
			t.Fatal(observation)
		}
	}

	observations[0].Status.Ready = false
	if !d.cachedRuntimeStatuses(targets[:1])[0].Status.Ready {
		t.Fatal("caller mutated cached readiness")
	}
}

func TestCachedStatusFencesPlacementAndExpiresObservations(t *testing.T) {
	d := &DeploymentService{}
	d.cacheStatus("first-node", model.RuntimeStatus{ServerID: "game", State: model.RuntimeStateRunning, Ready: true})
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	for _, tc := range []struct {
		node  string
		at    time.Time
		known bool
	}{
		{"first-node", now, true}, {"other-node", now, false}, {"first-node", now.Add(runtimeStatusLifetime), false}, {"first-node", now.Add(-time.Hour), false},
	} {
		status, observed := d.cachedStatusLocked("game", tc.node, tc.at)
		if (status.State == model.RuntimeStateRunning) != tc.known || (!observed.IsZero()) != tc.known {
			t.Fatal(tc, status, observed)
		}
	}
}
