package repository_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/integrations"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

func integrationEventStore(t *testing.T) (*database.Database, *repository.IntegrationRepository) {
	t.Helper()
	db, store, _ := integrationStore(t)

	binding, err := store.GetBinding(t.Context(), "binding")
	if err != nil {
		t.Fatal(err)
	}

	binding.Subscriptions = []integrations.Subscription{{Category: integrations.GameEvents}, {Category: integrations.PowerEvents}, {Category: integrations.ConsoleAlerts}, {Category: integrations.BackupEvents}}
	if err := store.UpdateBinding(t.Context(), binding); err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Model(&database.Server{}).Where("id = ?", "first").Update("game_integration", true).Error; err != nil {
		t.Fatal(err)
	}

	return db, store
}

func outgoingEvents(t *testing.T, db *database.Database) []database.IntegrationMessage {
	t.Helper()

	var messages []database.IntegrationMessage
	if err := db.ORM().Order("id").Find(&messages).Error; err != nil {
		t.Fatal(err)
	}

	return messages
}

func TestIntegrationRoutingIsProviderIndependentFilteredAndImmutable(t *testing.T) {
	db, store := integrationEventStore(t)

	ctx, now := t.Context(), time.Now()
	if err := store.CreateConnection(ctx, integrations.Connection{ID: "other", Provider: "fake-provider", Name: "Other", Enabled: true, Config: json.RawMessage(`{}`)}); err != nil {
		t.Fatal(err)
	}

	for _, binding := range []integrations.Binding{
		{ID: "filtered", IntegrationID: "other", ServerID: "first", DestinationID: "another", Enabled: true, Subscriptions: []integrations.Subscription{{Category: integrations.GameEvents, Rules: []integrations.EventRule{{Type: "player.joined", Fields: map[string]string{"player": "Alex", "ready": "false", "players": "0"}}}}}},
		{ID: "isolated", IntegrationID: "other", ServerID: "second", DestinationID: "another", Enabled: true, Subscriptions: []integrations.Subscription{{Category: integrations.GameEvents}}},
	} {
		if err := store.CreateBinding(ctx, binding); err != nil {
			t.Fatal(err)
		}
	}

	event := integrations.Event{ID: "game:one", ServerID: "first", Category: integrations.GameEvents, Type: "player.joined", Title: "Joined", OccurredAt: now.Unix(), Data: map[string]any{"player": "Alex", "ready": false, "players": 0}}
	if err := store.RouteEvent(ctx, event, now); err != nil {
		t.Fatal(err)
	}

	before := outgoingEvents(t, db)
	if len(before) != 2 {
		t.Fatal("provider fan-out failed", before)
	}

	event.Title = "Replay must not replace the payload"
	if err := store.RouteEvent(ctx, event, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	after := outgoingEvents(t, db)
	if len(after) != 2 || string(after[0].Payload) != string(before[0].Payload) || after[0].NextAttemptAt != before[0].NextAttemptAt {
		t.Fatal("replay mutated pending messages")
	}

	event.ID, event.Data["player"] = "game:two", "Alexandra"
	if err := store.RouteEvent(ctx, event, now); err != nil {
		t.Fatal(err)
	}

	if len(outgoingEvents(t, db)) != 3 {
		t.Fatal("field filter did not reject substring")
	}

	binding, err := store.GetBinding(ctx, "binding")
	if err != nil {
		t.Fatal(err)
	}

	binding.Enabled = false
	if err := store.UpdateBinding(ctx, binding); err != nil {
		t.Fatal(err)
	}

	connection, err := store.GetConnection(ctx, "other")
	if err != nil {
		t.Fatal(err)
	}

	connection.Enabled = false
	if err := store.UpdateConnection(ctx, connection); err != nil {
		t.Fatal(err)
	}

	event.ID, event.Data["player"] = "game:three", "Alex"
	if err := store.RouteEvent(ctx, event, now); err != nil {
		t.Fatal(err)
	}

	if len(outgoingEvents(t, db)) != 3 {
		t.Fatal("disabled routes queued messages")
	}
}

func TestGameIngestionAndIntegrationRoutingCommitTogether(t *testing.T) {
	db, _ := integrationEventStore(t)
	store, now := repository.NewGameEventRepository(db.ORM()), time.Now().UTC()

	event := model.GameEvent{ID: "joined", Type: "player.joined", Title: "Joined", OccurredAt: now, Data: map[string]any{"player": "Alex"}}

	event.FieldLabels = model.GameFieldLabels{"player": "Player name"}
	for range 2 {
		if err := store.Append(t.Context(), "node", "first", event, now, 24*time.Hour, 100); err != nil {
			t.Fatal(err)
		}
	}

	if len(outgoingEvents(t, db)) != 1 {
		t.Fatal("game replay duplicated messages")
	}

	var queued integrations.Event
	if err := json.Unmarshal(outgoingEvents(t, db)[0].Payload, &queued); err != nil || queued.FieldLabels["player"] != "Player name" {
		t.Fatal("notification lost dictionary labels", queued, err)
	}

	if err := store.Append(t.Context(), "wrong-node", "first", event, now, 24*time.Hour, 100); !errors.Is(err, repository.ErrGameEventPlacement) {
		t.Fatal("placement fence lost", err)
	}

	if err := db.ORM().Callback().Create().Before("gorm:create").Register("test:fail-integration-message", func(tx *gorm.DB) {
		if tx.Statement.Table == "integration_messages" {
			_ = tx.AddError(errors.New("simulated queue failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}

	event.ID = "must-roll-back"

	event.FieldLabels["player"] = "Must roll back"
	if err := store.Append(t.Context(), "node", "first", event, now, 24*time.Hour, 100); err == nil {
		t.Fatal("queue failure was acknowledged")
	}

	var count int64
	if err := db.ORM().Model(&database.GameEvent{}).Where("event_id = ?", event.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("source committed without outgoing message", count, err)
	}

	var label database.GameEventFieldLabel
	if err := db.ORM().Where("server_id = ? AND event_type = ? AND field_key = ?", "first", event.Type, "player").Take(&label).Error; err != nil || label.Label != "Player name" {
		t.Fatal("failed ingestion changed dictionary", label, err)
	}
}

func TestConsoleAlertsQueueOnlySafeGuidanceOnce(t *testing.T) {
	db, _ := integrationEventStore(t)
	alert := model.DetectedLogAlert{ID: fmt.Sprintf("%064x", 1), RuleKey: fmt.Sprintf("%064x", 2), ExecutionID: fmt.Sprintf("%064x", 3), Phase: model.LogRuntime, Title: "Check configuration", Message: "Review settings", Severity: model.LogError, DetectedAt: time.Now().Unix(), Line: "password=do-not-publish"}

	observation := model.LogObservation{Executions: map[model.LogPhase]string{model.LogRuntime: alert.ExecutionID}, Alerts: []model.DetectedLogAlert{alert}}
	for range 2 {
		if err := repository.NewLogAlertRepository(db.ORM()).Observe(t.Context(), "node", "first", observation, false); err != nil {
			t.Fatal(err)
		}
	}

	messages := outgoingEvents(t, db)
	if len(messages) != 1 || strings.Contains(string(messages[0].Payload), "do-not-publish") || !strings.Contains(string(messages[0].Payload), "Review settings") {
		t.Fatal("unsafe or duplicated console event", messages)
	}
}

func TestConfirmedOperationResultsRoutePowerAndBackupsWithoutSecrets(t *testing.T) {
	db, _ := integrationEventStore(t)

	store, now := repository.NewActivityRepository(db.ORM()), time.Now()
	for _, check := range []struct {
		action   model.ServerAction
		state    model.ExecutionState
		category integrations.Category
	}{
		{model.ServerActionStart, model.ExecutionSucceeded, integrations.PowerEvents},
		{model.ServerActionRestart, model.ExecutionInterrupted, integrations.PowerEvents},
		{model.ServerActionBackup, model.ExecutionSucceeded, integrations.BackupEvents},
		{model.ServerActionRestore, model.ExecutionFailed, integrations.BackupEvents},
	} {
		run := database.ScheduleRun{ID: string(check.action), ServerID: "first", ServerName: "First", Action: check.action, State: model.ExecutionRunning, Content: "secret-command", BackupMode: model.BackupModeSavedata, CreatedAt: now.Unix()}
		if err := store.CreateRun(t.Context(), &run); err != nil {
			t.Fatal(err)
		}

		before := len(outgoingEvents(t, db))
		for range 2 {
			if err := store.FinishRun(t.Context(), run, check.state, "secret-runtime-error", now); err != nil {
				t.Fatal(err)
			}
		}

		messages := outgoingEvents(t, db)
		if len(messages) != before+1 {
			t.Fatal("operation completion duplicated or missing")
		}

		message := messages[len(messages)-1]
		if message.Category != check.category || strings.Contains(string(message.Payload), "secret-") || !strings.Contains(string(message.Payload), string(check.state)) {
			t.Fatal("unsafe or incorrect operation outcome", message)
		}
	}
}

func TestPowerSamplingRequiresFreshConfirmedChangesAndSkipsReplay(t *testing.T) {
	db, _ := integrationEventStore(t)
	store := repository.NewResourceHistoryRepository(db.ORM())

	base := time.Now().Unix()
	for index, check := range []struct {
		state        model.RuntimeState
		started, age int64
		messages     int
	}{
		{model.RuntimeStateStopped, 0, 0, 0},
		{model.RuntimeStateStarting, 0, 0, 0},
		{model.RuntimeStateRunning, base, 0, 1},
		{model.RuntimeStateRunning, base, 0, 1},
		{model.RuntimeStateRunning, base + 4, 0, 2},
		{model.RuntimeStateStopped, 0, 60, 2},
		{model.RuntimeStateUnknown, 0, 0, 2},
		{model.RuntimeStateRunning, base + 7, 0, 2},
		{model.RuntimeStateStopped, 0, 0, 3},
	} {
		at := base + int64(index)

		sample := database.ServerResourceSample{ServerID: "first", SparkID: "node", CollectedAt: at, ReceivedAt: at, Usage: model.ResourceUsage{State: check.state, StartedAt: check.started, SampledAt: at - check.age, SampleAgeSeconds: check.age}}
		for range 2 {
			if err := store.Save(t.Context(), []database.ServerResourceSample{sample}, nil); err != nil {
				t.Fatal(err)
			}
		}

		if got := len(outgoingEvents(t, db)); got != check.messages {
			t.Fatalf("sample %d (%s): got %d messages, want %d", index, check.state, got, check.messages)
		}
	}
}
