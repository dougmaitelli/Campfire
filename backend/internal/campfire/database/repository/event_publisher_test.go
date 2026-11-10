package repository_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/events"
	"campfire/internal/campfire/integrations"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

func TestMonitoringPublicationRetainsItsSourceAndRollsBackEveryFollowUp(t *testing.T) {
	for _, failedTable := range []string{"monitoring_alerts", "notification_recipients", "notification_deliveries"} {
		t.Run(failedTable, func(t *testing.T) {
			db, _ := integrationEventStore(t)
			if err := db.ORM().Create(&database.User{ID: 1, Username: "admin", RoleID: database.AdminRole, CredentialVersion: 1}).Error; err != nil {
				t.Fatal(err)
			}

			store := repository.NewNotificationRepository(db.ORM())
			now := time.Now()
			transition := func(state *database.AlertState) *events.MonitoringAlertChanged {
				state.TargetType, state.TargetID, state.LastObservedAt = model.NotificationServer, "first", now.Unix()
				if state.Active {
					return nil
				}

				state.Active = true

				return &events.MonitoringAlertChanged{ID: "cpu-raised", TargetType: model.NotificationServer, TargetID: "first", Name: "First", Code: "CPU", Body: "CPU usage exceeded the configured threshold.", Active: true}
			}
			failure := errors.New("monitoring follow-up unavailable")

			if err := db.ORM().Callback().Create().Before("gorm:create").Register("test:monitoring-publication", func(tx *gorm.DB) {
				if tx.Statement.Table == failedTable {
					_ = tx.AddError(failure)
				}
			}); err != nil {
				t.Fatal(err)
			}

			if err := store.Observe(t.Context(), "server:first:CPU", true, transition); !errors.Is(err, failure) {
				t.Fatal(err)
			}

			for _, table := range []any{&database.AlertState{}, &database.MonitoringAlert{}, &database.Notification{}, &database.NotificationRecipient{}, &database.NotificationDelivery{}} {
				var count int64
				if err := db.ORM().Model(table).Count(&count).Error; err != nil || count != 0 {
					t.Fatal("partial monitoring publication", table, count, err)
				}
			}

			if err := db.ORM().Callback().Create().Remove("test:monitoring-publication"); err != nil {
				t.Fatal(err)
			}

			for range 2 {
				if err := store.Observe(t.Context(), "server:first:CPU", true, transition); err != nil {
					t.Fatal(err)
				}
			}

			activity := repository.NewServerActivityRepository(db.ORM())

			page, err := activity.List(t.Context(), "first", repository.ServerActivityAccess{}, repository.ServerActivityFilter{})
			if err != nil || len(page.Items) != 1 || page.Items[0].Key != "monitoring_alert:cpu-raised" || page.Items[0].NotificationCount != 1 {
				t.Fatal(page, err)
			}

			details, err := activity.Details(t.Context(), "first", page.Items[0].Key, repository.ServerActivityAccess{})
			if err != nil || details.MonitoringAlert == nil {
				t.Fatal(details, err)
			}

			notifications, err := activity.Notifications(t.Context(), "first", details.Key, repository.ServerActivityAccess{}, "")
			if err != nil || len(notifications.Items) != 1 || !notifications.Items[0].SourceAvailable {
				t.Fatal(notifications, err)
			}
			// Source retention is independent of the inbox and its deliveries.
			if _, err := repository.NewActivityRepository(db.ORM()).PruneHistory(t.Context(), time.Time{}, now.Add(time.Hour)); err != nil {
				t.Fatal(err)
			}

			page, err = activity.List(t.Context(), "first", repository.ServerActivityAccess{}, repository.ServerActivityFilter{})
			if err != nil || len(page.Items) != 1 || page.Items[0].Kind != "notification" || page.Items[0].SourceAvailable {
				t.Fatal(page, err)
			}
		})
	}
}

func TestJobPublicationAuditsLifecycleAndRollsBackItsCheckpoint(t *testing.T) {
	db, _ := integrationEventStore(t)
	ctx := t.Context()
	store := repository.NewJobRepository(db.ORM())

	job := database.ServerJob{ID: "published-job", Type: model.JobCopy, SourceServerID: "first", TargetServerID: "second", SourceSparkID: "node", TargetSparkID: "node", Status: model.JobPending, Step: model.JobPreparing, Attempt: 1, CreatedAt: time.Now().Unix()}
	if err := store.Create(ctx, &job); err != nil {
		t.Fatal(err)
	}

	if err := store.Advance(ctx, job, map[string]any{"status": model.JobRunning}); err != nil {
		t.Fatal(err)
	}

	job, err := store.Get(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}

	failure := errors.New("job audit unavailable")

	if err := db.ORM().Callback().Create().Before("gorm:create").Register("test:job-publication", func(tx *gorm.DB) {
		if tx.Statement.Table == "audit_events" {
			_ = tx.AddError(failure)
		}
	}); err != nil {
		t.Fatal(err)
	}

	if err := store.Advance(ctx, job, map[string]any{"status": model.JobFailed}); !errors.Is(err, failure) {
		t.Fatal(err)
	}

	unchanged, err := store.Get(ctx, job.ID)
	if err != nil || unchanged.Status != model.JobRunning {
		t.Fatal("audit escaped its job checkpoint", unchanged, err)
	}

	if err := db.ORM().Callback().Create().Remove("test:job-publication"); err != nil {
		t.Fatal(err)
	}

	if err := store.Advance(ctx, job, map[string]any{"status": model.JobFailed}); err != nil {
		t.Fatal(err)
	}

	if err := store.Advance(ctx, job, map[string]any{"status": model.JobFailed}); !errors.Is(err, repository.ErrJobState) {
		t.Fatal("stale transition published again", err)
	}

	var audits []database.AuditEvent
	if err := db.ORM().Find(&audits).Error; err != nil || len(audits) != 1 || audits[0].Action != "job.copy.failed" || audits[0].Outcome != model.AuditFailed {
		t.Fatal(audits, err)
	}
	// Jobs do not silently acquire new inbox or integration delivery policy.
	var notes, messages int64
	if err := db.ORM().Model(&database.Notification{}).Count(&notes).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Model(&database.IntegrationMessage{}).Where("category <> ?", integrations.CardCategory).Count(&messages).Error; err != nil || notes != 0 || messages != 0 {
		t.Fatal(notes, messages, err)
	}
}

func TestApplicationPublicationRollsBackSourceAndEveryFollowUp(t *testing.T) {
	for _, failedTable := range []string{"notification_recipients", "notification_deliveries", "integration_messages"} {
		t.Run(failedTable, func(t *testing.T) {
			db, _ := integrationEventStore(t)
			if err := db.ORM().Create(&database.User{ID: 1, Username: "admin", RoleID: database.AdminRole, CredentialVersion: 1}).Error; err != nil {
				t.Fatal(err)
			}

			failure := errors.New("follow-up storage unavailable")

			if err := db.ORM().Callback().Create().Before("gorm:create").Register("test:publication-failure", func(tx *gorm.DB) {
				if tx.Statement.Table == failedTable {
					_ = tx.AddError(failure)
				}
			}); err != nil {
				t.Fatal(err)
			}

			alert := model.DetectedLogAlert{ID: fmt.Sprintf("%064x", 1), RuleKey: fmt.Sprintf("%064x", 2), ExecutionID: fmt.Sprintf("%064x", 3), Phase: model.LogRuntime, Title: "Review configuration", Message: "Check the server settings.", Line: "private-token=never-publish", Severity: model.LogError, DetectedAt: time.Now().Unix()}
			observation := model.LogObservation{Executions: map[model.LogPhase]string{model.LogRuntime: alert.ExecutionID}, Alerts: []model.DetectedLogAlert{alert}}

			store := repository.NewLogAlertRepository(db.ORM())
			if err := store.Observe(t.Context(), "node", "first", observation, true); !errors.Is(err, failure) {
				t.Fatal("publication failure was hidden", err)
			}

			for _, table := range []any{&database.LogAlertIssue{}, &database.Notification{}, &database.NotificationRecipient{}, &database.NotificationDelivery{}, &database.IntegrationMessage{}} {
				var count int64
				if err := db.ORM().Model(table).Count(&count).Error; err != nil || count != 0 {
					t.Fatalf("partial publication in %T: %d, %v", table, count, err)
				}
			}

			if err := db.ORM().Callback().Create().Remove("test:publication-failure"); err != nil {
				t.Fatal(err)
			}

			for range 2 {
				if err := store.Observe(t.Context(), "node", "first", observation, true); err != nil {
					t.Fatal(err)
				}
			}

			for _, table := range []any{&database.LogAlertIssue{}, &database.Notification{}, &database.NotificationRecipient{}, &database.NotificationDelivery{}, &database.IntegrationMessage{}} {
				var count int64
				if err := db.ORM().Model(table).Count(&count).Error; err != nil || count != 1 {
					t.Fatalf("recovery duplicated or lost %T: %d, %v", table, count, err)
				}
			}

			var note database.Notification
			if err := db.ORM().First(&note).Error; err != nil || note.SourceKey != "console_alert:"+alert.ID || note.Severity != model.NotificationFailure || strings.Contains(note.Body, "private-token") {
				t.Fatal("unsafe or unlinked panel notification", note, err)
			}

			message := outgoingEvents(t, db)[0]
			if message.SourceKey != note.SourceKey || strings.Contains(string(message.Payload), "private-token") {
				t.Fatal("unsafe or unlinked integration notification", message)
			}
		})
	}
}

func TestApplicationPublicationPreservesNotificationPolicies(t *testing.T) {
	db, _ := integrationEventStore(t)
	publisher := repository.NewEventPublisher(db.ORM(), true)
	now := time.Now().UTC()

	game := events.GameEventRecorded{ServerID: "first", RecordID: 42, Event: model.GameEvent{ID: "joined", Type: "player.joined", Title: "Alex joined", OccurredAt: now, Data: map[string]any{"player": "Alex"}}}
	for _, event := range []events.Event{
		game,
		events.GameMediaRecorded{GameEventRecorded: game, Media: model.GameMedia{ID: "image", Name: "shot.png", ContentType: "image/png", Size: 5}},
		events.OperationFinished{ServerID: "first", OperationID: "backup", Action: model.ServerActionBackup, State: model.ExecutionSucceeded, BackupMode: model.BackupModeSavedata},
		events.OperationFinished{ServerID: "first", OperationID: "command", Action: model.ServerActionCommand, State: model.ExecutionSucceeded},
		events.PowerStateChanged{ServerID: "first", Previous: model.RuntimeStateStopped, Current: model.RuntimeStateRunning},
	} {
		result, err := publisher.Publish(t.Context(), event, now)
		if err != nil || result.NotificationID != 0 {
			t.Fatal("integration-only event changed inbox policy", result, err)
		}
	}

	messages := outgoingEvents(t, db)
	if len(messages) != 4 || messages[0].SourceKey != events.GameEventSourceKey(game.RecordID) || messages[1].SourceKey != messages[0].SourceKey {
		t.Fatal("routing identities or policies changed", messages)
	}

	result, err := publisher.Publish(t.Context(), events.NotificationRequested{TargetType: model.NotificationServer, TargetID: "first", Kind: model.NotificationHealth, Severity: model.NotificationWarning, Title: "Resource alert", Body: "Current resource guidance."}, now)
	if err != nil || result.NotificationID < 1 || len(outgoingEvents(t, db)) != 4 {
		t.Fatal("panel notification was forwarded as another integration message", result, err)
	}

	var note database.Notification
	if err := db.ORM().First(&note, result.NotificationID).Error; err != nil || note.CreatedAt != now.Unix() || note.Title != "Resource alert" {
		t.Fatal("panel notification changed", note, err)
	}

	var delivery database.NotificationDelivery
	if err := db.ORM().First(&delivery, "notification_id = ?", note.ID).Error; err != nil || delivery.State != model.DeliveryPending {
		t.Fatal("configured Apprise delivery was lost", delivery, err)
	}

	result, err = publisher.Publish(t.Context(), nil, now)
	if !errors.Is(err, repository.ErrUnsupportedEvent) || result.NotificationID != 0 {
		t.Fatal("unsupported publication silently accepted", result, err)
	}
}

func TestApplicationPublicationRespectsOuterOperationTransaction(t *testing.T) {
	db, _ := integrationEventStore(t)
	store, now := repository.NewActivityRepository(db.ORM()), time.Now()

	run := database.ScheduleRun{ID: "atomic-operation", ServerID: "first", ServerName: "First", Action: model.ServerActionRestart, State: model.ExecutionRunning, CreatedAt: now.Unix()}
	if err := store.CreateRun(t.Context(), &run); err != nil {
		t.Fatal(err)
	}

	failure := errors.New("audit storage unavailable")

	if err := db.ORM().Callback().Create().Before("gorm:create").Register("test:audit-failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "audit_events" {
			_ = tx.AddError(failure)
		}
	}); err != nil {
		t.Fatal(err)
	}

	if err := store.FinishRun(t.Context(), run, model.ExecutionSucceeded, "Completed", now); !errors.Is(err, failure) {
		t.Fatal("outer transaction failure was hidden", err)
	}

	var retained database.ScheduleRun
	if err := db.ORM().First(&retained, "id = ?", run.ID).Error; err != nil || retained.State != model.ExecutionRunning || len(outgoingEvents(t, db)) != 0 {
		t.Fatal("publication escaped its source transaction", retained, err)
	}

	if err := db.ORM().Callback().Create().Remove("test:audit-failure"); err != nil {
		t.Fatal(err)
	}

	if err := store.FinishRun(t.Context(), run, model.ExecutionSucceeded, "Completed", now); err != nil || len(outgoingEvents(t, db)) != 1 {
		t.Fatal("operation could not recover once audit storage returned", err)
	}
}
