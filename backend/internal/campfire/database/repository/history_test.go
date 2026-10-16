package repository_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
)

func historyStore(t *testing.T) (*database.Database, *repository.ActivityRepository) {
	t.Helper()

	db, err := database.Open(context.Background(), testDatabaseSource(t))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	return db, repository.NewActivityRepository(db.ORM())
}

func TestHistoryPruningPreservesActiveAndRecentlyFinishedRecords(t *testing.T) {
	db, store := historyStore(t)
	ctx := context.Background()
	cutoff := time.Now().UTC().Truncate(time.Second).Add(-30 * 24 * time.Hour)

	old := cutoff.Add(-time.Hour).Unix()
	for _, state := range []string{"succeeded", "failed", "interrupted", "cancelled", "queued", "dispatching", "running", "unknown"} {
		if err := store.CreateRun(ctx, &database.ScheduleRun{ID: state, State: state, CreatedAt: old, FinishedAt: old}); err != nil {
			t.Fatal(err)
		}
	}

	for _, item := range []database.ScheduleRun{
		{ID: "recent-finish", State: "succeeded", CreatedAt: old, FinishedAt: cutoff.Add(time.Hour).Unix()},
		{ID: "at-cutoff", State: "failed", CreatedAt: old, FinishedAt: cutoff.Unix()},
		{ID: "no-finish", State: "succeeded", CreatedAt: old},
	} {
		if err := store.CreateRun(ctx, &item); err != nil {
			t.Fatal(err)
		}
	}

	for _, outcome := range []string{"succeeded", "failed", "accepted", "interrupted", "cancelled", "pending", "unknown"} {
		if err := store.BeginAudit(ctx, &database.AuditEvent{ID: outcome, Outcome: outcome, CreatedAt: old}); err != nil {
			t.Fatal(err)
		}
	}

	if err := store.BeginAudit(ctx, &database.AuditEvent{ID: "at-cutoff", Outcome: "succeeded", CreatedAt: cutoff.Unix()}); err != nil {
		t.Fatal(err)
	}

	if more, err := store.PruneHistory(ctx, time.Time{}, time.Time{}); err != nil || more {
		t.Fatal("disabled retention failed", err)
	}

	var count int64
	if err := db.ORM().Model(&database.ScheduleRun{}).Count(&count).Error; err != nil || count != 11 {
		t.Fatal("disabled retention changed records", count, err)
	}

	if more, err := store.PruneHistory(ctx, time.Time{}, cutoff); err != nil || more {
		t.Fatal(err)
	}

	if err := db.ORM().Model(&database.AuditEvent{}).Count(&count).Error; err != nil || count != 8 {
		t.Fatal("operation policy changed audit history", count, err)
	}

	runs, err := store.Runs(ctx, 1, true, "", "")
	if err != nil || len(runs) != 7 {
		t.Fatal("terminal pruning failed", len(runs), err)
	}

	for _, run := range runs {
		if run.ID == "succeeded" || run.ID == "failed" || run.ID == "interrupted" || run.ID == "cancelled" {
			t.Fatal("expired result retained", run.ID)
		}
	}

	if more, err := store.PruneHistory(ctx, cutoff, time.Time{}); err != nil || more {
		t.Fatal(err)
	}

	audit, err := store.Audit(ctx, "", "", "")
	if err != nil || len(audit) != 3 {
		t.Fatal("audit pruning failed", len(audit), err)
	}

	if err := db.ORM().Model(&database.ScheduleRun{}).Count(&count).Error; err != nil || count != 7 {
		t.Fatal("audit policy changed operations", count, err)
	}
	// A request that was pending during cleanup can still record its outcome.
	if err := store.FinishAudit(ctx, database.AuditEvent{ID: "pending", Outcome: "succeeded"}); err != nil {
		t.Fatal(err)
	}

	if _, err := store.PruneHistory(ctx, cutoff, time.Time{}); err != nil {
		t.Fatal(err)
	}

	audit, err = store.Audit(ctx, "", "", "")
	if err != nil || len(audit) != 2 {
		t.Fatal("completed request did not become eligible", len(audit), err)
	}
}

func TestHistoryPruningIsBatchedAndCursorsSurviveAnchorDeletion(t *testing.T) {
	db, store := historyStore(t)
	ctx := context.Background()
	cutoff := time.Now().UTC().Truncate(time.Second)

	items := make([]database.AuditEvent, 1005)
	for i := range items {
		items[i] = database.AuditEvent{ID: fmt.Sprintf("audit-%04d", i), Outcome: "succeeded", CreatedAt: cutoff.Add(-time.Hour).Unix()}
	}

	if err := db.ORM().CreateInBatches(items, 100).Error; err != nil {
		t.Fatal(err)
	}

	if more, err := store.PruneHistory(ctx, cutoff, time.Time{}); err != nil || !more {
		t.Fatal("backlog not reported", err)
	}

	var count int64
	if err := db.ORM().Model(&database.AuditEvent{}).Count(&count).Error; err != nil || count != 5 {
		t.Fatal("batch was not bounded", count, err)
	}

	if more, err := store.PruneHistory(ctx, cutoff, time.Time{}); err != nil || more {
		t.Fatal("backlog did not finish", err)
	}

	for _, item := range []database.ScheduleRun{
		{ID: "anchor", State: "succeeded", CreatedAt: cutoff.Add(-time.Hour).Unix(), FinishedAt: cutoff.Add(-time.Minute).Unix()},
		{ID: "retained", State: "succeeded", CreatedAt: cutoff.Add(-2 * time.Hour).Unix(), FinishedAt: cutoff.Add(time.Hour).Unix()},
	} {
		if err := store.CreateRun(ctx, &item); err != nil {
			t.Fatal(err)
		}
	}

	cursor := fmt.Sprintf("%d:anchor", cutoff.Add(-time.Hour).Unix())
	if _, err := store.PruneHistory(ctx, time.Time{}, cutoff); err != nil {
		t.Fatal(err)
	}

	runs, err := store.Runs(ctx, 1, true, "", cursor)
	if err != nil || len(runs) != 1 || runs[0].ID != "retained" {
		t.Fatal("pruning the cursor lost retained history", len(runs), err)
	}

	for _, cursor := range []string{"invalid:id", "-1:id", "1:"} {
		if _, err := store.Runs(ctx, 1, true, "", cursor); err == nil {
			t.Fatal("invalid cursor accepted", cursor)
		}
	}
}
