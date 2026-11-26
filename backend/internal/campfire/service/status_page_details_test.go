package service

import (
	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/statuspage"
	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
	"testing"
	"time"
)

func TestPublicGameDetailsUseStoredObservationsAndCurrentPlacement(t *testing.T) {
	for _, scenario := range []string{"fresh", "old update", "future update", "stale sample", "sample error", "old usage placement", "old game placement", "old runtime placement", "disabled integration", "disabled spark", "disconnected", "invalidated", "stopped"} {
		t.Run(scenario, func(t *testing.T) {
			activity, db, _, _ := activityFixture(t)
			d := activity.deployment
			online := &activityConnectivity{}
			online.online.Store(scenario != "disconnected")
			d.Sparks.connectivity = online
			remote := &historyRuntime{}
			d.Remote = remote

			if err := db.ORM().Model(&database.Server{}).Where("id = ?", "game").Update("game_integration", true).Error; err != nil {
				t.Fatal(err)
			}

			now := time.Now()
			d.Games = &GameIntegrationService{Servers: d.Servers, now: func() time.Time { return now }}

			frame := protocol.GameFrame{ServerID: "game", Publication: model.GamePublication{Version: 1, State: &model.GameState{Players: new(3), Fields: map[string]any{"player_names": []any{"Alex", "Sam"}, "map": "world"}, FieldLabels: model.GameFieldLabels{"map": "World name"}}}}
			if err := d.Games.Receive(t.Context(), "node", frame); err != nil {
				t.Fatal(err)
			}

			d.cacheStatus("node", model.RuntimeStatus{ServerID: "game", State: model.RuntimeStateRunning, Ready: true})
			sample := database.ServerResourceSample{ServerID: "game", SparkID: "node", CollectedAt: now.Unix(), ReceivedAt: now.Unix(), Usage: model.ResourceUsage{ServerID: "game", State: model.RuntimeStateRunning, SampledAt: now.Unix()}}
			record := d.Games.states["game"]

			switch scenario {
			case "old update":
				record.received = now.Add(-2 * time.Minute)
			case "future update":
				record.received = now.Add(time.Hour)
			case "stale sample":
				sample.ReceivedAt = now.Add(-time.Hour).Unix()
			case "sample error":
				sample.Usage.ContainerError = "unavailable"
			case "old usage placement":
				sample.SparkID = "previous-node"
			case "old game placement":
				record.sparkID = "previous-node"
			case "old runtime placement":
				d.statusSparks["game"] = "previous-node"
			case "disabled integration":
				if err := db.ORM().Model(&database.Server{}).Where("id = ?", "game").Update("game_integration", false).Error; err != nil {
					t.Fatal(err)
				}
			case "disabled spark":
				if err := db.ORM().Model(&database.Spark{}).Where("id = ?", "node").Update("disabled", true).Error; err != nil {
					t.Fatal(err)
				}
			case "stopped":
				d.cacheStatus("node", model.RuntimeStatus{ServerID: "game", State: model.RuntimeStateStopped})
			}

			d.Games.states["game"] = record
			if scenario == "invalidated" {
				d.Games.Invalidate("game")
			}

			if err := d.History.Store.Save(t.Context(), []database.ServerResourceSample{sample}, nil); err != nil {
				t.Fatal(err)
			}

			store := repository.NewStatusPageRepository(db.ORM())
			if _, err := store.Save(t.Context(), "", statuspage.Input{Title: "Community", Slug: "community", Entries: []statuspage.Selection{{ServerID: "game", Name: "Public game"}}}); err != nil {
				t.Fatal(err)
			}

			service := &StatusPageService{Store: store, Deployment: d}

			page, err := service.Public(t.Context(), "community")
			if err != nil || len(page.Entries) != 1 {
				t.Fatal(page, err)
			}

			view := page.Entries[0].GameState

			want := scenario == "fresh" || scenario == "old update"
			if (view != nil && view.State != nil) != want {
				t.Fatal("incorrect public game data", scenario, view)
			}

			if want {
				if view.UpdatedAt != record.received.Unix() || *view.State.Players != 3 || view.State.Fields["map"] != "world" || view.State.FieldLabels["map"] != "World name" {
					t.Fatal("snapshot changed", view)
				}

				view.State.Fields["map"] = "mutated"
				if d.Games.states["game"].state.Fields["map"] != "world" {
					t.Fatal("public caller mutated cache")
				}
			}

			if remote.calls.Load() != 0 {
				t.Fatal("anonymous read contacted runtime")
			}
		})
	}
}
