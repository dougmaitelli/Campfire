package service

import (
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	core "campfire/internal/campfire/integrations"
	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
)

func TestIntegrationCardUsesCurrentPlacementAndMonitoringFreshnessWithoutNetwork(t *testing.T) {
	activity, db, _, _ := activityFixture(t)
	deployment := activity.deployment
	online := &activityConnectivity{}
	online.online.Store(true)
	deployment.Sparks.connectivity = online
	runtime := &historyRuntime{}
	deployment.Remote = runtime

	if err := db.ORM().Model(&database.Server{}).Where("id = ?", "game").Update("game_integration", true).Error; err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	games := &GameIntegrationService{
		Servers: deployment.Servers, now: func() time.Time { return now },
		Events: NewGameEventService(repository.NewGameEventRepository(db.ORM())),
	}

	deployment.Games = games
	if err := games.register(t.Context(), "node", "game", model.GameRegistration{
		Revision: 1, StateFields: model.GameFieldLabels{"invite_code": "Invite code", "player_names": "Online players"},
	}); err != nil {
		t.Fatal(err)
	}

	publication := protocol.GameFrame{ServerID: "game", Publication: model.GamePublication{Version: 1, State: &model.GameState{Players: new(3), Fields: map[string]any{"invite_code": "ABCD", "player_names": []any{"Alex", "Sam"}, "private": []any{"unselected"}}}}}
	settings := core.CardSettings{Fields: []string{"players", "field.invite_code", "field.player_names"}}

	for _, scenario := range []string{"fresh", "expired publication", "stale sample", "sample error", "disconnected", "old placement", "stopped", "stale status", "invalidated", "disabled"} {
		t.Run(scenario, func(t *testing.T) {
			online.online.Store(scenario != "disconnected")

			games.blocked = nil
			if err := games.Receive(t.Context(), "node", publication); err != nil {
				t.Fatal(err)
			}

			state := model.RuntimeStateRunning
			if scenario == "stopped" {
				state = model.RuntimeStateStopped
			}

			deployment.cacheStatus("node", model.RuntimeStatus{ServerID: "game", State: state})
			sample := database.ServerResourceSample{ServerID: "game", SparkID: "node", CollectedAt: now.Unix(), ReceivedAt: now.Unix(), Usage: model.ResourceUsage{ServerID: "game", State: model.RuntimeStateRunning, SampledAt: now.Unix()}}

			switch scenario {
			case "expired publication":
				record := games.states["game"]
				record.received = now.Add(-gameStateFreshFor)
				games.states["game"] = record
			case "stale sample":
				sample.ReceivedAt = now.Add(-time.Hour).Unix()
			case "sample error":
				sample.Usage.ContainerError = "unavailable"
			case "old placement":
				deployment.statusSparks["game"] = "previous-node"
			case "stale status":
				deployment.observed["game"] = now.Add(-runtimeStatusLifetime)
			case "invalidated":
				games.Invalidate("game")
			}

			if err := deployment.History.Store.Save(t.Context(), []database.ServerResourceSample{sample}, nil); err != nil {
				t.Fatal(err)
			}

			if err := db.ORM().Model(&database.Server{}).Where("id = ?", "game").Update("game_integration", scenario != "disabled").Error; err != nil {
				t.Fatal(err)
			}

			card, err := deployment.IntegrationCard(t.Context(), "game", settings)
			if err != nil || len(card.Fields) != 3 || card.Fields[0].Available != (scenario == "fresh") || card.Fields[1].Available != (scenario == "fresh") || card.Fields[2].Available != (scenario == "fresh") {
				t.Fatal("incorrect current game values", card, err)
			}

			if card.Fields[1].Name != "Invite code" || card.Fields[2].Name != "Online players" {
				t.Fatal("durable labels disappeared with unavailable values", scenario, card)
			}

			if scenario == "fresh" && (card.Fields[0].Value != "3" || card.Fields[1].Value != "ABCD" || card.Fields[2].Value != "Alex, Sam") {
				t.Fatal(card)
			}

			if scenario == "fresh" && (card.ExpiresAt <= now.Unix() || card.ExpiresAt > now.Add(runtimeStatusLifetime).Unix()) {
				t.Fatal("card can outlive its source observations", card)
			}

			if (scenario == "old placement" || scenario == "stale status") && card.State != model.RuntimeStateUnknown {
				t.Fatal("old runtime state exposed", card)
			}
		})
	}

	if runtime.calls.Load() != 0 {
		t.Fatal("card reads contacted Spark", runtime.calls.Load())
	}
}
