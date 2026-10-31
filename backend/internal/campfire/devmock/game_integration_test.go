//go:build !production

package devmock

import (
	"slices"
	"testing"
	"time"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
)

func TestGameSimulationIngestionLifecycleAndReplay(t *testing.T) {
	runtime, db, _, servers := fixture(t)
	server := onlineServer(t, runtime, servers)
	store := repository.NewGameEventRepository(db.ORM())
	games := &service.GameIntegrationService{Servers: repository.NewServerRepository(db.ORM()), Events: service.NewGameEventService(store)}
	games.Media = service.NewGameMediaService(repository.NewGameMediaRepository(db.ORM()))
	runtime.games = games
	now := time.Now()
	tick := func(seconds int) {
		t.Helper()

		if err := runtime.gameTick(t.Context(), now.Add(time.Duration(seconds)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	command := func(action, content string) {
		t.Helper()

		status, err := runtime.Action(t.Context(), server.SparkID, model.RuntimeRequest{ServerID: server.ID, Action: model.ServerAction(action), Content: content})
		if err != nil {
			t.Fatal(err)
		}

		if status.Busy {
			settleRuntime(t, runtime, server)
		}
	}
	snapshot := func(players, fields int) {
		t.Helper()

		view := games.Snapshot(server.ID, server.SparkID, true, time.Now().Unix())
		if view == nil || view.Status != "fresh" || view.State == nil || *view.State.Players != players || len(view.State.Fields) != fields {
			t.Fatalf("unexpected snapshot: %+v", view)
		}

		if view.State.Capacity == nil || *view.State.Capacity != 20 {
			t.Fatal("missing capacity", view.State)
		}
	}
	events := func(kind string) int {
		t.Helper()

		page, err := store.List(t.Context(), server.ID, repository.GameEventQuery{Type: kind, Limit: 100})
		if err != nil {
			t.Fatal(err)
		}

		return len(page.Items)
	}

	tick(0)
	snapshot(0, 1)

	baseline := events("")

	tick(5)

	if events("") != baseline {
		t.Fatal("heartbeat created an event")
	}

	tick(15)
	snapshot(1, 0)

	joined := events("")
	if joined != baseline+1 {
		t.Fatal("join was not persisted")
	}

	command("command", "mock-game replay")

	if events("") != joined {
		t.Fatal("replay duplicated durable event")
	}

	command("command", "mock-game pause")
	tick(90)

	view := games.Snapshot(server.ID, server.SparkID, true, time.Now().Add(61*time.Second).Unix())
	if view == nil || view.Status != "stale" || view.State != nil || events("") != joined {
		t.Fatal("paused producer did not expire cleanly", view)
	}

	command("command", "mock-game resume")
	tick(95)
	snapshot(1, 0)

	mediaPage, err := store.List(t.Context(), server.ID, repository.GameEventQuery{Limit: 1})
	if err != nil || len(mediaPage.Items) != 1 || len(mediaPage.Items[0].Attachments) != 2 {
		t.Fatal("mock publisher did not ingest its screenshot and clip", mediaPage, err)
	}

	command("command", "mock-game next")
	tick(96)
	snapshot(0, 1)

	if events("player.left") != 1 {
		t.Fatal("leave was not persisted")
	}

	games.Invalidate(server.ID)
	command("stop", "")

	stopped := events("")

	tick(120)

	if events("") != stopped {
		t.Fatal("stopped game published events")
	}

	if view := games.Snapshot(server.ID, server.SparkID, false, time.Now().Unix()); view == nil || view.State != nil {
		t.Fatal("stopped game exposed state", view)
	}

	command("start", "")
	games.Observe(server.ID, server.SparkID, model.ResourceUsage{State: "running"}, time.Now(), false)
	tick(125)
	snapshot(0, 1)

	if events("") != stopped+1 {
		t.Fatal("new session reused old event identity")
	}

	_, _, features := runtime.SparkFeatures(server.SparkID)
	for _, capability := range []string{protocol.CapabilityGameIntegration, protocol.CapabilityGameEvents} {
		if !slices.Contains(features, capability) {
			t.Fatal("missing simulated capability", capability)
		}
	}
}

func TestGameSimulationWorkerStopsBeforeDatabaseClose(t *testing.T) {
	runtime, db, _, _ := fixture(t)
	games := &service.GameIntegrationService{Servers: repository.NewServerRepository(db.ORM()), Events: service.NewGameEventService(repository.NewGameEventRepository(db.ORM()))}
	closeSimulation := runtime.StartGameSimulation(t.Context(), games)
	closeSimulation()
}

func TestGameSimulationRetriesOriginalEventAndSkipsOfflineServers(t *testing.T) {
	runtime, db, _, servers := fixture(t)
	server := onlineServer(t, runtime, servers)
	store := repository.NewGameEventRepository(db.ORM())
	events := service.NewGameEventService(store)
	runtime.games = &service.GameIntegrationService{Servers: repository.NewServerRepository(db.ORM()), Events: events}
	now := time.Now()
	events.MaxPerServer = 0

	if err := runtime.gameTick(t.Context(), now); err == nil {
		t.Fatal("ingestion failure was hidden")
	}

	session := runtime.gameSessions[server.ID]

	original := *session.last
	if session.step != 0 {
		t.Fatal("failed delivery advanced the scenario")
	}

	events.MaxPerServer = 10000

	if err := runtime.gameTick(t.Context(), now.Add(5*time.Second)); err != nil {
		t.Fatal(err)
	}

	page, err := store.List(t.Context(), server.ID, repository.GameEventQuery{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}

	if page.Items[0].EventID != original.ID || !page.Items[0].OccurredAt.Equal(original.OccurredAt) {
		t.Fatal("retry changed event identity or occurrence time", page.Items[0])
	}

	for _, candidate := range servers {
		if !runtime.Connected(candidate.SparkID) {
			if runtime.gameSessions[candidate.ID] != nil || runtime.games.Snapshot(candidate.ID, candidate.SparkID, true, now.Unix()) != nil {
				t.Fatal("offline Spark published state", candidate.ID)
			}
		}
	}
}
