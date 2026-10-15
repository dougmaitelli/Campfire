package service

import (
	"context"
	"testing"
	"time"

	"campfire/internal/campfire/database"
)

func TestHistoryRetentionWorkerPreservesSchedulesAndRecentRecovery(t *testing.T) {
	s, db, _, owner := activityFixture(t)
	s.Retention = HistoryRetention{AuditDays: 1, OperationDays: 1}

	schedule := savedSchedule(t, s, owner, "backup")
	if err := db.ORM().Model(&database.Schedule{}).Where("id = ?", schedule.ID).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}

	old := time.Now().Add(-48 * time.Hour).Unix()
	for _, item := range []database.ScheduleRun{
		{ID: "expired", ServerID: "game", State: "succeeded", CreatedAt: old, FinishedAt: old},
		{ID: "recover", ServerID: "game", State: "dispatching", CreatedAt: old},
	} {
		if err := s.Store.CreateRun(context.Background(), &item); err != nil {
			t.Fatal(err)
		}
	}

	if err := s.Store.BeginAudit(context.Background(), &database.AuditEvent{ID: "expired", Outcome: "succeeded", CreatedAt: old}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(s.Close)

	deadline := time.Now().Add(3 * time.Second)

	for {
		var count int64
		if err := db.ORM().Model(&database.ScheduleRun{}).Where("id = ?", "expired").Count(&count).Error; err != nil {
			t.Fatal(err)
		}

		if count == 0 {
			break
		}

		if time.Now().After(deadline) {
			t.Fatal("background retention did not run")
		}

		time.Sleep(10 * time.Millisecond)
	}

	run, err := s.Store.Run(ctx, "recover")
	if err != nil || run.State != "interrupted" || run.FinishedAt <= old {
		t.Fatal("recent recovery result was deleted", err)
	}

	if _, err := s.Store.Schedule(ctx, schedule.ID); err != nil {
		t.Fatal("retention deleted schedule definition", err)
	}

	audit, err := s.Store.Audit(ctx, "", "", "")
	if err != nil {
		t.Fatal(err)
	}

	for _, item := range audit {
		if item.ID == "expired" {
			t.Fatal("audit worker did not prune")
		}
	}

	cancel()
	s.Close()
}

func TestHistoryRetentionValidation(t *testing.T) {
	for _, config := range []HistoryRetention{{AuditDays: -1}, {OperationDays: -1}, {AuditDays: 36501}, {OperationDays: 36501}} {
		if config.Validate() == nil {
			t.Fatal("invalid history age accepted", config)
		}
	}

	for _, config := range []HistoryRetention{{}, {AuditDays: 36500, OperationDays: 1}} {
		if err := config.Validate(); err != nil {
			t.Fatal(err)
		}
	}
}
