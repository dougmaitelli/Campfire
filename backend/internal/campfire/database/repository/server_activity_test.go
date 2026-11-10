package repository_test

import (
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

func TestServerActivityGroupsGameMediaAndPreservesIndependentRetention(t *testing.T) {
	db, _ := integrationEventStore(t)
	store := repository.NewServerActivityRepository(db.ORM())
	access := repository.ServerActivityAccess{Categories: []integrations.Category{integrations.GameEvents}}
	now := time.Now()

	event := model.GameEvent{ID: "capture", Type: "game.capture", Title: "World captured", OccurredAt: now.Add(24 * time.Hour), Data: map[string]any{"player": "Alex"}, FieldLabels: model.GameFieldLabels{"player": "Player name"}}
	if err := repository.NewGameEventRepository(db.ORM()).Append(t.Context(), "node", "first", event, now, time.Hour, 20); err != nil {
		t.Fatal(err)
	}

	if err := repository.NewGameMediaRepository(db.ORM()).Append(t.Context(), "node", "first", event.ID, model.GameMedia{ID: "image", Name: "shot.png", ContentType: "image/png", Size: 5}, []byte("image")); err != nil {
		t.Fatal(err)
	}

	if err := repository.NewGameMediaRepository(db.ORM()).Append(t.Context(), "node", "first", event.ID, model.GameMedia{ID: "video", Name: "clip.webm", ContentType: "video/webm", Size: 5}, []byte("video")); err != nil {
		t.Fatal(err)
	}

	page, err := store.List(t.Context(), "first", access, repository.ServerActivityFilter{})
	if err != nil || len(page.Items) != 1 || page.Items[0].Kind != "game" || page.Items[0].NotificationCount != 3 || page.Items[0].CreatedAt != now.Unix() {
		t.Fatal("grouping or receipt-order time failed", page, err)
	}

	details, err := store.Details(t.Context(), "first", page.Items[0].Key, access)
	if err != nil || details.GameEvent == nil || len(details.GameEvent.Attachments) != 2 || details.GameEvent.Attachments[0].Data != nil {
		t.Fatal("detail lost media or notification links", details, err)
	}

	page, err = store.Notifications(t.Context(), "first", details.Key, access, "")
	if err != nil || len(page.Items) != 3 {
		t.Fatal("source-scoped query hid destination records", page, err)
	}

	attachments := map[string]string{}

	for _, item := range page.Items {
		if !item.SourceAvailable || item.SourceKey != details.Key {
			t.Fatal("missing reverse source link", item)
		}

		attachments[item.AttachmentName] = item.AttachmentContentType
	}

	if len(attachments) != 3 || attachments[""] != "" || attachments["shot.png"] != "image/png" || attachments["clip.webm"] != "video/webm" {
		t.Fatal("notification content cannot be distinguished", attachments)
	}

	if err := db.ORM().Where("server_id = ?", "first").Delete(&database.GameEvent{}).Error; err != nil {
		t.Fatal(err)
	}

	page, err = store.List(t.Context(), "first", access, repository.ServerActivityFilter{})
	if err != nil || len(page.Items) != 3 || page.Items[0].SourceAvailable {
		t.Fatal("source retention erased notification history", page, err)
	}

	page, err = store.List(t.Context(), "second", access, repository.ServerActivityFilter{})
	if err != nil || len(page.Items) != 0 {
		t.Fatal("activity escaped the server scope", page, err)
	}
}

func TestServerActivityFiltersAndCursorApplyBeforePagination(t *testing.T) {
	db, _ := integrationEventStore(t)
	store := repository.NewServerActivityRepository(db.ORM())
	access := repository.ServerActivityAccess{}

	for index := range 75 {
		item := database.Notification{TargetType: model.NotificationServer, TargetID: "first", Kind: model.NotificationAlert, Severity: model.NotificationInfo, Title: fmt.Sprintf("Notice %03d", index), Body: "public guidance", CreatedAt: 100}
		if err := db.ORM().Create(&item).Error; err != nil {
			t.Fatal(err)
		}
	}

	first, err := store.List(t.Context(), "first", access, repository.ServerActivityFilter{})
	if err != nil || len(first.Items) != 50 || first.Next == "" {
		t.Fatal(first, err)
	}

	second, err := store.List(t.Context(), "first", access, repository.ServerActivityFilter{Before: first.Next})
	if err != nil || len(second.Items) != 25 || second.Next != "" {
		t.Fatal(second, err)
	}

	seen := map[string]bool{}
	for _, item := range append(first.Items, second.Items...) {
		if seen[item.Key] {
			t.Fatal("cursor repeated an item", item.Key)
		}

		seen[item.Key] = true
	}

	filtered, err := store.List(t.Context(), "first", access, repository.ServerActivityFilter{Search: "Notice 07", From: 100, To: 101})
	if err != nil || len(filtered.Items) != 5 {
		t.Fatal("filters ran after pagination", filtered, err)
	}

	if _, err := store.List(t.Context(), "first", access, repository.ServerActivityFilter{Before: "bad-cursor"}); err == nil {
		t.Fatal("invalid cursor accepted")
	}

	source := database.MonitoringAlert{ID: "source", TargetType: model.NotificationServer, TargetID: "first", Kind: model.NotificationAlert, CreatedAt: 99}
	if err := db.ORM().Create(&source).Error; err != nil {
		t.Fatal(err)
	}

	key := "monitoring_alert:" + source.ID
	if err := db.ORM().Model(&database.Notification{}).Where("target_id = ?", "first").Update("source_key", key).Error; err != nil {
		t.Fatal(err)
	}

	unrelated := database.Notification{TargetType: model.NotificationServer, TargetID: "first", SourceKey: "monitoring_alert:other", Title: "Other source", CreatedAt: 200}
	if err := db.ORM().Create(&unrelated).Error; err != nil {
		t.Fatal(err)
	}

	first, err = store.Notifications(t.Context(), "first", key, access, "")
	if err != nil || len(first.Items) != 50 || first.Next == "" {
		t.Fatal("first related page lost records", first, err)
	}

	second, err = store.Notifications(t.Context(), "first", key, access, first.Next)
	if err != nil || len(second.Items) != 25 || second.Next != "" {
		t.Fatal("older related page lost records", second, err)
	}

	for _, item := range append(first.Items, second.Items...) {
		if item.SourceKey != key || !item.SourceAvailable {
			t.Fatal("related pages escaped their source", item)
		}
	}

	if _, err := store.Notifications(t.Context(), "second", key, access, ""); !errors.Is(err, repository.ErrAccessNotFound) {
		t.Fatal("related notifications escaped the server scope", err)
	}
}

func TestServerActivityRedactsEvidenceAndRestrictedOperationsBeforeCounting(t *testing.T) {
	db, _ := integrationEventStore(t)
	store := repository.NewServerActivityRepository(db.ORM())

	issue := database.LogAlertIssue{ID: "alert", ServerID: "first", SparkID: "node", ExecutionID: "execution", Phase: model.LogRuntime, DetectedAt: 100, ReceivedAt: 100, Detection: model.DetectedLogAlert{ID: "alert", ExecutionID: "execution", Phase: model.LogRuntime, Title: "Safe summary", Message: "Safe guidance", Line: "private secret", DetectedAt: 100}}
	if err := db.ORM().Omit("Server").Create(&issue).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Create(&database.ScheduleRun{ID: "operation", ServerID: "first", Action: model.ServerActionBackup, State: model.ExecutionSucceeded, CreatedAt: 99}).Error; err != nil {
		t.Fatal(err)
	}

	access := repository.ServerActivityAccess{ManualOperations: true}

	page, err := store.List(t.Context(), "first", access, repository.ServerActivityFilter{})
	if err != nil || len(page.Items) != 1 {
		t.Fatal("restricted operation exposed in activity", page, err)
	}

	details, err := store.Details(t.Context(), "first", "console_alert:alert", access)
	if err != nil || details.Alert == nil || details.Alert.Line != "" {
		t.Fatal("console evidence leaked", details, err)
	}

	page, err = store.List(t.Context(), "first", access, repository.ServerActivityFilter{Search: "private secret"})
	if err != nil || len(page.Items) != 0 {
		t.Fatal("search revealed protected evidence", page, err)
	}

	access.Console, access.Backups = true, true

	page, err = store.List(t.Context(), "first", access, repository.ServerActivityFilter{})
	if err != nil || len(page.Items) != 2 {
		t.Fatal("permitted operations missing", page, err)
	}
}

func TestActivityPanelRetryOnlyChangesTheRequestedServersDelivery(t *testing.T) {
	db, _ := integrationEventStore(t)
	store := repository.NewServerActivityRepository(db.ORM())
	ids := make([]int64, 0, 2)

	for _, serverID := range []string{"first", "second"} {
		item := database.Notification{TargetType: model.NotificationServer, TargetID: serverID, Kind: model.NotificationAlert, Severity: model.NotificationFailure, Title: "Alert", CreatedAt: 100}
		if err := db.ORM().Create(&item).Error; err != nil {
			t.Fatal(err)
		}

		if err := db.ORM().Omit("Notification").Create(&database.NotificationDelivery{NotificationID: item.ID, State: model.DeliveryFailed, Attempts: 8, Error: "Delivery failed"}).Error; err != nil {
			t.Fatal(err)
		}

		ids = append(ids, item.ID)
	}

	if err := store.RetryPanelDelivery(t.Context(), "first", ids[1], time.Now()); err == nil {
		t.Fatal("retry crossed server boundary")
	}

	if err := store.RetryPanelDelivery(t.Context(), "first", ids[0], time.Now()); err != nil {
		t.Fatal(err)
	}

	var untouched database.NotificationDelivery
	if err := db.ORM().Take(&untouched, "notification_id = ?", ids[1]).Error; err != nil || untouched.State != model.DeliveryFailed || untouched.Attempts != 8 {
		t.Fatal("retry changed another server", untouched, err)
	}
}

func TestServerActivityUsesStablePanelReceiptForAlerts(t *testing.T) {
	db, _ := integrationEventStore(t)
	store := repository.NewServerActivityRepository(db.ORM())
	alert := model.DetectedLogAlert{ID: strings.Repeat("a", 64), RuleKey: strings.Repeat("b", 64), ExecutionID: strings.Repeat("c", 64), Phase: model.LogRuntime, Title: "Action needed", Message: "Review settings", Severity: model.LogError, DetectedAt: time.Now().Add(24 * time.Hour).Unix()}
	observation := model.LogObservation{Executions: map[model.LogPhase]string{model.LogRuntime: alert.ExecutionID}, Alerts: []model.DetectedLogAlert{alert}}
	before := time.Now().Unix()

	alerts := repository.NewLogAlertRepository(db.ORM())
	if err := alerts.Observe(t.Context(), "node", "first", observation, false); err != nil {
		t.Fatal(err)
	}

	page, err := store.List(t.Context(), "first", repository.ServerActivityAccess{}, repository.ServerActivityFilter{})
	if err != nil || len(page.Items) != 1 || page.Items[0].CreatedAt < before || page.Items[0].CreatedAt > time.Now().Unix() {
		t.Fatal("activity used the Spark clock", page, err)
	}

	received := page.Items[0].CreatedAt
	// A replay and independent inbox retention must not change the receipt.
	if err := db.ORM().Where("target_id = ?", "first").Delete(&database.Notification{}).Error; err != nil {
		t.Fatal(err)
	}

	if err := alerts.Observe(t.Context(), "node", "first", observation, false); err != nil {
		t.Fatal(err)
	}

	details, err := store.Details(t.Context(), "first", page.Items[0].Key, repository.ServerActivityAccess{})
	if err != nil || details.CreatedAt != received || details.Alert == nil || details.Alert.DetectedAt != alert.DetectedAt {
		t.Fatal("receipt or original detection changed", details, err)
	}
}

func TestServerActivityDiscoveryIsScopedAndCachesImmutableHistory(t *testing.T) {
	db, _ := integrationEventStore(t)
	store := repository.NewServerActivityRepository(db.ORM())
	game := repository.NewGameEventRepository(db.ORM())
	now := time.Now()

	event := model.GameEvent{ID: "first-event", Type: "player.joined", Title: "Joined", OccurredAt: now, Data: map[string]any{"player": "Alex"}, FieldLabels: model.GameFieldLabels{"player": "Player name"}}
	if err := game.Append(t.Context(), "node", "first", event, now, time.Hour, 20); err != nil {
		t.Fatal(err)
	}

	scans := 0

	if err := db.ORM().Callback().Row().Before("gorm:row").Register("test:discovery-scans", func(tx *gorm.DB) {
		if tx.Statement.Table == "game_events" && strings.Join(tx.Statement.Selects, ",") == "type,data" {
			scans++
		}
	}); err != nil {
		t.Fatal(err)
	}

	for _, kind := range []string{"", "operations", "alert"} {
		choices, err := store.FilterChoices(t.Context(), "first", kind)
		if err != nil || len(choices.Game.Types) != 0 || scans != 0 {
			t.Fatal("unrelated view scanned game data", kind, scans, choices, err)
		}
	}

	for range 2 {
		choices, err := store.FilterChoices(t.Context(), "first", "game")
		if err != nil || scans != 1 || len(choices.Game.Fields["player"]) != 1 || choices.Game.FieldLabels[event.Type]["player"] != "Player name" {
			t.Fatal("unchanged history was rescanned or choices lost", scans, choices, err)
		}
	}

	event.ID, event.Data["player"], event.FieldLabels["player"] = "new-event", "Morgan", "Member"
	// Keep count constant while replacing the retained event.
	if err := game.Append(t.Context(), "node", "first", event, now.Add(time.Second), time.Hour, 1); err != nil {
		t.Fatal(err)
	}

	choices, err := store.FilterChoices(t.Context(), "first", "game")
	if err != nil || scans != 2 || len(choices.Game.Fields["player"]) != 1 || choices.Game.Fields["player"][0] != "Morgan" || choices.Game.FieldLabels[event.Type]["player"] != "Member" {
		t.Fatal("ingestion did not invalidate choices and labels", scans, choices, err)
	}

	if err := db.ORM().Where("server_id = ?", "first").Delete(&database.GameEvent{}).Error; err != nil {
		t.Fatal(err)
	}

	choices, err = store.FilterChoices(t.Context(), "first", "game")
	if err != nil || scans != 3 || len(choices.Game.Types) != 0 || len(choices.Game.Fields) != 0 {
		t.Fatal("retention did not invalidate choices", scans, choices, err)
	}
}

func TestServerActivitySearchMatchesDisplayedGameValues(t *testing.T) {
	db, _ := integrationEventStore(t)
	game := repository.NewGameEventRepository(db.ORM())
	store := repository.NewServerActivityRepository(db.ORM())

	now := time.Now()
	for _, event := range []model.GameEvent{
		{ID: "escaped", Type: "game.message", Title: "Announcement", OccurredAt: now, Data: map[string]any{"path": `C:\saves\world`, "message": "Status <ready> \"quoted\" Line\nbreak 50%_complete!"}},
		{ID: "encoded-title", Type: "game.message", Title: `Literal \u003cready\u003e`, OccurredAt: now, Data: map[string]any{}},
	} {
		if err := game.Append(t.Context(), "node", "first", event, now, time.Hour, 20); err != nil {
			t.Fatal(err)
		}
	}
	// Non-game sources must retain plain-text search semantics.
	note := database.Notification{TargetType: model.NotificationServer, TargetID: "first", Title: "Monitor guidance", Body: "Status <ready>", CreatedAt: now.Unix()}
	if err := db.ORM().Create(&note).Error; err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		search    string
		game, all int
	}{
		{`C:\saves`, 1, 1},
		{"<ready>", 1, 2},
		{`"quoted"`, 1, 1},
		{"Line\nbreak", 1, 1},
		{"50%_complete!", 1, 1},
		{"Announcement", 1, 1},
		{"missing", 0, 0},
	} {
		t.Run(test.search, func(t *testing.T) {
			legacy, err := game.List(t.Context(), "first", repository.GameEventQuery{Search: test.search, Limit: 50})
			if err != nil || len(legacy.Items) != test.game {
				t.Fatal("game search failed", legacy, err)
			}

			for _, kind := range []string{"", "game"} {
				want := test.all
				if kind == "game" {
					want = test.game
				}

				page, err := store.List(t.Context(), "first", repository.ServerActivityAccess{}, repository.ServerActivityFilter{Kind: kind, Search: test.search})
				if err != nil || len(page.Items) != want {
					t.Fatal("Activity search differed from displayed values", kind, want, page, err)
				}
			}
		})
	}
}

func TestServerActivityDeliveryStatesMatchIntegrationHistory(t *testing.T) {
	db, outbox := integrationEventStore(t)
	ctx, now := t.Context(), time.Now()
	activity := repository.NewServerActivityRepository(db.ORM())
	categories := []integrations.Category{integrations.GameEvents}
	access := repository.ServerActivityAccess{Categories: categories}

	event := model.GameEvent{ID: "delivery-states", Type: "game.announcement", Title: "Announcement", OccurredAt: now}
	if err := repository.NewGameEventRepository(db.ORM()).Append(ctx, "node", "first", event, now, time.Hour, 20); err != nil {
		t.Fatal(err)
	}

	sources, err := activity.List(ctx, "first", access, repository.ServerActivityFilter{})
	if err != nil || len(sources.Items) != 1 || sources.Items[0].Kind != "game" {
		t.Fatal("missing source activity", sources, err)
	}

	for _, state := range []model.DeliveryState{
		model.DeliveryPending, model.DeliveryDelivered, model.DeliveryFailed,
		integrations.MessageClaimed, integrations.MessageSending, integrations.MessageReconciling,
		integrations.MessageUncertain, integrations.MessageGlobalUncertain,
		integrations.MessageRateLimited, integrations.MessageGlobalLimited,
		integrations.MessageCredentialsBlocked, integrations.MessageDestinationBlocked,
	} {
		t.Run(string(state), func(t *testing.T) {
			if err := db.ORM().Model(&database.IntegrationMessage{}).Where("binding_id = ?", "binding").Update("state", state).Error; err != nil {
				t.Fatal(err)
			}

			history, err := outbox.MessageStatuses(ctx, "binding", categories, 0, "")
			if err != nil || len(history) != 1 {
				t.Fatal(history, err)
			}

			page, err := activity.Notifications(ctx, "first", sources.Items[0].Key, access, "")
			if err != nil || len(page.Items) != 1 || page.Items[0].State != history[0].State {
				t.Fatal("Activity status or filtering differs from integration history", page, history, err)
			}
		})
	}
}

func TestServerActivityDistinguishesScheduledDeliveryChecks(t *testing.T) {
	db, outbox := integrationEventStore(t)
	ctx := t.Context()
	now := time.Now().Truncate(time.Second)
	previous := now.Add(-10 * time.Minute)
	activity := repository.NewServerActivityRepository(db.ORM())
	access := repository.ServerActivityAccess{Categories: []integrations.Category{integrations.GameEvents}}

	event := model.GameEvent{ID: "receipt", Type: "game.announcement", Title: "Announcement", OccurredAt: previous}
	if err := repository.NewGameEventRepository(db.ORM()).Append(ctx, "node", "first", event, previous, time.Hour, 20); err != nil {
		t.Fatal(err)
	}

	sources, err := activity.List(ctx, "first", access, repository.ServerActivityFilter{})
	if err != nil || len(sources.Items) != 1 || sources.Items[0].Kind != "game" {
		t.Fatal("missing source activity", sources, err)
	}

	claim, err := outbox.ClaimMessage(ctx, previous)
	if err != nil || claim == nil {
		t.Fatal(claim, err)
	}

	if err := outbox.BeginMessageSend(ctx, *claim); err != nil {
		t.Fatal(err)
	}

	assertCheck := func(state string, pending bool) {
		t.Helper()

		page, err := activity.Notifications(ctx, "first", sources.Items[0].Key, access, "")
		if err != nil || len(page.Items) != 1 || page.Items[0].State != state || page.Items[0].DeliveryCheckPending != pending {
			t.Fatal("incorrect delivery check in feed", page, err)
		}

		details, err := activity.Details(ctx, "first", page.Items[0].Key, access)
		if err != nil || details.State != state || details.DeliveryCheckPending != pending {
			t.Fatal("incorrect delivery check in details", details, err)
		}
	}

	binding, err := outbox.GetBinding(ctx, claim.BindingID)
	if err != nil {
		t.Fatal(err)
	}

	setEnabled := func(enabled bool) {
		t.Helper()

		binding.Enabled = enabled
		if err := outbox.UpdateBinding(ctx, binding); err != nil {
			t.Fatal(err)
		}
	}

	due := previous.Add(10 * time.Second)
	if err := outbox.FinishMessage(ctx, *claim, integrations.MessageUncertain, "", "Receipt unavailable", due, previous); err != nil {
		t.Fatal(err)
	}

	assertCheck("uncertain", true)
	setEnabled(false)
	assertCheck("uncertain", false)
	setEnabled(true)

	claim, err = outbox.ClaimMessage(ctx, due)
	if err != nil || claim == nil || !claim.Reconcile {
		t.Fatal(claim, err)
	}

	assertCheck("uncertain", true)
	// This claim's lease has elapsed, so disabling the binding pauses recovery.
	setEnabled(false)
	assertCheck("uncertain", false)
	setEnabled(true)

	if err := outbox.FinishMessage(ctx, *claim, integrations.MessageUncertain, "", "Receipt not found", time.Time{}, due); err != nil {
		t.Fatal(err)
	}

	assertCheck("uncertain", false)

	if err := outbox.RetryMessage(ctx, claim.BindingID, claim.ID, access.Categories, now); err != nil {
		t.Fatal(err)
	}

	claim, err = outbox.ClaimMessage(ctx, now)
	if err != nil || claim == nil || !claim.Reconcile {
		t.Fatal(claim, err)
	}

	setEnabled(false)
	assertCheck("uncertain", true)

	if err := outbox.FinishMessage(ctx, *claim, model.DeliveryDelivered, "300", "", time.Time{}, now); err != nil {
		t.Fatal(err)
	}

	assertCheck("delivered", false)
}
