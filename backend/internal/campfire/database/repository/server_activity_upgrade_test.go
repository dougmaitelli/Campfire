package repository_test

import (
	"encoding/json"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	core "campfire/internal/campfire/integrations"
	"campfire/internal/shared/model"
)

func TestServerActivityUpgradePreservesHistory(t *testing.T) {
	db, store, source := integrationStore(t)
	ctx := t.Context()

	note := database.Notification{TargetType: model.NotificationServer, TargetID: "first", Title: "Retained", Body: "Original body", CreatedAt: 100}
	if err := db.ORM().Create(&note).Error; err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		id       string
		category core.Category
		want     string
	}{
		{"console_alert:retained", core.ConsoleAlerts, "console_alert:retained"},
		{"operation:power", core.PowerEvents, "operation:power"},
		{"operation:backup", core.BackupEvents, "operation:backup"},
		{"unmatched-game-event", core.GameEvents, ""},
	}
	for _, test := range tests {
		if _, err := store.EnqueueMessage(ctx, "binding", test.id, test.category, json.RawMessage(`{"title":"Retained"}`), time.Unix(100, 0)); err != nil {
			t.Fatal(err)
		}
	}

	for _, id := range []string{"linked", "legacy"} {
		issue := database.LogAlertIssue{ID: id, ServerID: "first", SparkID: "node", Phase: model.LogRuntime, DetectedAt: 200}
		if err := db.ORM().Omit("Server").Create(&issue).Error; err != nil {
			t.Fatal(err)
		}
	}

	linked := database.Notification{TargetType: model.NotificationServer, TargetID: "first", SourceKey: "console_alert:linked", Title: "Linked", CreatedAt: 150}
	if err := db.ORM().Create(&linked).Error; err != nil {
		t.Fatal(err)
	}
	// Migrate receipt times independently of whether source-key columns exist.
	if err := db.ORM().Migrator().DropColumn(&database.LogAlertIssue{}, "ReceivedAt"); err != nil {
		t.Fatal(err)
	}

	for _, table := range []any{&database.Notification{}, &database.IntegrationMessage{}} {
		if err := db.ORM().Migrator().DropIndex(table, "SourceKey"); err != nil {
			t.Fatal(err)
		}

		if err := db.ORM().Migrator().DropColumn(table, "SourceKey"); err != nil {
			t.Fatal(err)
		}
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	for range 2 {
		reopened, err := database.Open(ctx, source)
		if err != nil {
			t.Fatal(err)
		}

		for _, table := range []any{&database.Notification{}, &database.IntegrationMessage{}} {
			if !reopened.ORM().Migrator().HasColumn(table, "SourceKey") || !reopened.ORM().Migrator().HasIndex(table, "SourceKey") {
				t.Fatalf("missing source column/index: %T", table)
			}
		}

		var retained database.Notification
		if err := reopened.ORM().First(&retained, note.ID).Error; err != nil || retained.Title != note.Title || retained.Body != note.Body || retained.SourceKey != "" {
			t.Fatal("notification changed", retained, err)
		}

		for _, id := range []string{"linked", "legacy"} {
			var issue database.LogAlertIssue
			// The old schema has no source links; preserve its original time.
			if err := reopened.ORM().Take(&issue, "server_id = ? AND id = ?", "first", id).Error; err != nil || issue.ReceivedAt != 200 {
				t.Fatal("legacy receipt migration failed", issue, err)
			}
		}

		for _, test := range tests {
			var row database.IntegrationMessage
			if err := reopened.ORM().Take(&row, "event_id = ?", test.id).Error; err != nil || row.SourceKey != test.want || string(row.Payload) != `{"title":"Retained"}` {
				t.Fatal("history/source changed", row, test.want, err)
			}
		}

		if err := reopened.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestServerActivityReceiptUpgradePreservesExactLinkedTime(t *testing.T) {
	db, _, source := integrationStore(t)

	issue := database.LogAlertIssue{ID: "linked", ServerID: "first", SparkID: "node", Phase: model.LogRuntime, DetectedAt: 200}
	if err := db.ORM().Omit("Server").Create(&issue).Error; err != nil {
		t.Fatal(err)
	}

	for _, note := range []database.Notification{
		{TargetType: model.NotificationServer, TargetID: "first", SourceKey: "console_alert:linked", Title: "Linked", CreatedAt: 150},
		{TargetType: model.NotificationServer, TargetID: "second", SourceKey: "console_alert:linked", Title: "Other server", CreatedAt: 100},
	} {
		if err := db.ORM().Create(&note).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := db.ORM().Migrator().DropColumn(&database.LogAlertIssue{}, "ReceivedAt"); err != nil {
		t.Fatal(err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	for range 2 {
		reopened, err := database.Open(t.Context(), source)
		if err != nil {
			t.Fatal(err)
		}

		var retained database.LogAlertIssue
		if err := reopened.ORM().Take(&retained, "server_id = ? AND id = ?", "first", "linked").Error; err != nil || retained.ReceivedAt != 150 || retained.DetectedAt != 200 {
			t.Fatal("linked receipt migration failed", retained, err)
		}

		if err := reopened.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
