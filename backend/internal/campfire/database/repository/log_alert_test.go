package repository_test

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

func TestLogAlertReplaySkipsStoredInsertAttempts(t *testing.T) {
	db, _ := resourceHistoryStore(t)
	store := repository.NewLogAlertRepository(db.ORM())

	observation := model.LogObservation{Executions: map[model.LogPhase]string{model.LogRuntime: fmt.Sprintf("%064x", 2055)}}
	for index := range model.LogAlertHistoryLimit {
		rule, execution := fmt.Sprintf("%064x", index%32+1024), fmt.Sprintf("%064x", index/32+2048)
		id := fmt.Sprintf("%x", sha256.Sum256([]byte("game\x00"+execution+"\x00"+rule)))
		observation.Alerts = append(observation.Alerts, model.DetectedLogAlert{ID: id, RuleKey: rule, ExecutionID: execution, Phase: model.LogRuntime, Title: "Problem", Message: "Review settings", Severity: model.LogError, DetectedAt: time.Now().Unix()})
	}

	if err := store.Observe(t.Context(), "node", "game", observation, false); err != nil {
		t.Fatal(err)
	}

	writes := 0

	if err := db.ORM().Callback().Create().After("gorm:create").Register("test:log-alert-inserts", func(tx *gorm.DB) {
		if tx.Statement.Table == "log_alert_issues" {
			writes++
		}
	}); err != nil {
		t.Fatal(err)
	}

	if err := store.Observe(t.Context(), "node", "game", observation, false); err != nil {
		t.Fatal(err)
	}

	if writes != 0 {
		t.Fatalf("unchanged history issued %d insert attempts", writes)
	}

	observation.Alerts = append(observation.Alerts[1:], model.DetectedLogAlert{ID: strings.Repeat("f", 64), RuleKey: strings.Repeat("e", 64), ExecutionID: observation.Executions[model.LogRuntime], Phase: model.LogRuntime, Title: "New problem", Message: "Review settings", Severity: model.LogWarning, DetectedAt: time.Now().Unix()})
	if err := store.Observe(t.Context(), "node", "game", observation, false); err != nil {
		t.Fatal(err)
	}

	if writes != 1 {
		t.Fatalf("new detection should require one insert, got %d", writes)
	}

	var notifications int64
	if err := db.ORM().Model(&database.Notification{}).Count(&notifications).Error; err != nil || notifications != model.LogAlertHistoryLimit+1 {
		t.Fatal("replay duplicated or lost notifications", notifications, err)
	}
}

func TestLogAlertsPersistDeduplicateDismissAndFencePlacement(t *testing.T) {
	db, _ := resourceHistoryStore(t)
	store := repository.NewLogAlertRepository(db.ORM())
	alert := model.DetectedLogAlert{ID: strings.Repeat("a", 64), RuleKey: strings.Repeat("b", 64), ExecutionID: strings.Repeat("c", 64), Phase: model.LogRuntime, Title: "EULA requires acceptance", Message: "Review eula.txt", Severity: model.LogError, DetectedAt: time.Now().Unix(), Line: "private-token=secret", Action: &model.LogAlertAction{Type: model.LogOpenFile, Path: "eula.txt"}}
	observation := model.LogObservation{Executions: map[model.LogPhase]string{model.LogRuntime: alert.ExecutionID}, Alerts: []model.DetectedLogAlert{alert}}

	var workers sync.WaitGroup
	for range 8 {
		workers.Go(func() {
			if err := store.Observe(t.Context(), "node", "game", observation, true); err != nil {
				t.Error(err)
			}
		})
	}

	workers.Wait()

	var messages []database.Notification
	if err := db.ORM().Find(&messages).Error; err != nil || len(messages) != 1 {
		t.Fatal("replayed observations duplicated notifications", messages, err)
	}

	if strings.Contains(messages[0].Body, "secret") {
		t.Fatal("notification leaked console evidence")
	}

	items, err := store.List(t.Context(), "game", false)
	if err != nil || len(items) != 1 || items[0].Line != "" {
		t.Fatal("server-only viewing leaked logs", items, err)
	}

	items, err = store.List(t.Context(), "game", true)
	if err != nil || len(items) != 1 || items[0].Line != alert.Line {
		t.Fatal("console readers lost evidence", items, err)
	}

	if err := store.Dismiss(t.Context(), "other", alert.ID); err == nil {
		t.Fatal("dismissal crossed server boundaries")
	}

	if err := store.Dismiss(t.Context(), "game", alert.ID); err != nil {
		t.Fatal(err)
	}

	reopened := repository.NewLogAlertRepository(db.ORM())
	if err := reopened.Observe(t.Context(), "node", "game", observation, false); err != nil {
		t.Fatal(err)
	}

	items, err = reopened.List(t.Context(), "game", true)
	if err != nil || len(items) != 0 {
		t.Fatal("replay forgot dismissal", items, err)
	}

	if err := reopened.Observe(t.Context(), "wrong-spark", "game", observation, false); err == nil {
		t.Fatal("wrong placement accepted")
	}

	alert.ID, alert.ExecutionID = strings.Repeat("d", 64), strings.Repeat("e", 64)
	observation.Executions[model.LogRuntime] = alert.ExecutionID

	observation.Alerts = append(observation.Alerts, alert)
	if err := reopened.Observe(t.Context(), "node", "game", observation, false); err != nil {
		t.Fatal(err)
	}

	items, err = reopened.List(t.Context(), "game", true)
	if err != nil || len(items) != 1 || items[0].ID != alert.ID {
		t.Fatal("new execution did not rearm the issue", items, err)
	}

	observation.Executions[model.LogRuntime] = strings.Repeat("f", 64)
	if err := reopened.Observe(t.Context(), "node", "game", observation, false); err != nil {
		t.Fatal(err)
	}

	items, err = reopened.List(t.Context(), "game", true)
	if err != nil || len(items) != 0 {
		t.Fatal("previous execution remained active", items, err)
	}

	var count int64
	if err := db.ORM().Model(&database.Notification{}).Count(&count).Error; err != nil || count != 2 {
		t.Fatal("notification identity was not durable", count, err)
	}
}
