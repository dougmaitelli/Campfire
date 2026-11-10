package integrations

import (
	"math"
	"strings"
	"testing"
	"time"

	"campfire/internal/shared/model"
)

func TestGameEventIdentityPreservesOpaqueIDsAndPublisherClock(t *testing.T) {
	for _, id := range []string{"event", " event ", strings.Repeat(" ", 128)} {
		source := model.GameEvent{ID: id, Type: "player.joined", Title: "Joined", OccurredAt: time.Unix(-1, 0)}

		event := GameEvent("server", source)
		if err := event.Validate(); err != nil {
			t.Fatal("normalization narrowed the existing game publication contract", err)
		}

		if event.ID != GameEvent("server", source).ID || event.OccurredAt != -1 {
			t.Fatal("normalization lost source identity or time")
		}

		source.ID += "different"
		if event.ID == GameEvent("server", source).ID {
			t.Fatal("distinct source identities collapsed")
		}
	}
}

func TestSubscriptionMatchingUsesTypesAndExactScalarFields(t *testing.T) {
	event := Event{ID: "game:one", ServerID: "server", Category: GameEvents, Type: "player.joined", Title: "Joined", OccurredAt: time.Now().Unix(), Data: map[string]any{"player": "Alex", "ready": false, "players": float64(0), "empty": ""}}
	for _, check := range []struct {
		name         string
		subscription Subscription
		matches      bool
	}{
		{"all", Subscription{Category: GameEvents}, true},
		{"wrong category", Subscription{Category: PowerEvents}, false},
		{"type alternatives", Subscription{Category: GameEvents, Rules: []EventRule{{Type: "player.left"}, {Type: "player.joined"}}}, true},
		{"type mismatch", Subscription{Category: GameEvents, Rules: []EventRule{{Type: "player.left"}}}, false},
		{"exact fields", Subscription{Category: GameEvents, Rules: []EventRule{{Fields: map[string]string{"player": "alex", "ready": "false", "players": "0"}}}}, true},
		{"not a substring", Subscription{Category: GameEvents, Rules: []EventRule{{Fields: map[string]string{"player": "Al"}}}}, false},
		{"all fields required", Subscription{Category: GameEvents, Rules: []EventRule{{Fields: map[string]string{"player": "Alex", "missing": ""}}}}, false},
		{"empty means present", Subscription{Category: GameEvents, Rules: []EventRule{{Fields: map[string]string{"empty": ""}}}}, true},
	} {
		t.Run(check.name, func(t *testing.T) {
			if got := check.subscription.Matches(event); got != check.matches {
				t.Fatal(got)
			}
		})
	}

	if err := event.Validate(); err != nil {
		t.Fatal(err)
	}

	for _, invalid := range []any{math.NaN(), math.Inf(1), []string{"Alex"}, map[string]any{"name": "Alex"}, nil} {
		event.Data["player"] = invalid
		if err := event.Validate(); err == nil {
			t.Fatal("invalid payload value accepted", invalid)
		}
	}
}

func TestOperationEventsOnlyDescribeTerminalSupportedResults(t *testing.T) {
	for _, state := range []model.ExecutionState{model.ExecutionDispatching, model.ExecutionRunning} {
		if _, ok := OperationResult("server", "op", model.ServerActionStart, state, "", time.Now()); ok {
			t.Fatal("dispatch was advertised as complete")
		}
	}

	if _, ok := OperationResult("server", "op", model.ServerActionCommand, model.ExecutionSucceeded, "", time.Now()); ok {
		t.Fatal("console command became a notification")
	}

	for _, state := range []model.ExecutionState{model.ExecutionSucceeded, model.ExecutionFailed, model.ExecutionInterrupted, model.ExecutionSkipped} {
		event, ok := OperationResult("server", "op", model.ServerActionRestore, state, "", time.Now())
		if !ok || event.Category != BackupEvents || event.Data["state"] != string(state) || event.Type != "restore."+string(state) {
			t.Fatal(event, ok)
		}
	}
}

func TestEventRulesKeepFieldConditionsInsideTheirEventType(t *testing.T) {
	subscription := Subscription{Category: GameEvents, Rules: []EventRule{
		{Type: "game.announcement"},
		{Type: "player.joined", Fields: map[string]string{"player": "doug"}},
	}}
	for _, check := range []struct {
		kind string
		data map[string]any
		want bool
	}{
		{"game.announcement", nil, true},
		{"game.announcement", map[string]any{"player": "Alex"}, true},
		{"player.joined", map[string]any{"player": "Doug"}, true},
		{"player.joined", map[string]any{"player": "Alex"}, false},
		{"player.joined", nil, false},
		{"player.left", map[string]any{"player": "doug"}, false},
	} {
		if got := subscription.Matches(Event{Category: GameEvents, Type: check.kind, Data: check.data}); got != check.want {
			t.Fatal(check.kind, check.data, got)
		}
	}
}
