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

func TestScheduleHistoryExcludesDirectOperationsBeforePagination(t *testing.T) {
	_, store := historyStore(t)
	ctx := t.Context()

	for i := range 252 {
		run := database.ScheduleRun{ID: fmt.Sprintf("run-%03d", i), ServerID: "game", State: "succeeded", CreatedAt: int64(i + 1), Content: "private-command"}
		if i < 102 {
			// The original definition need not exist for its history to remain visible.
			run.ScheduleID = "deleted-schedule"
			run.Recurring = i%2 == 0

			if i == 101 {
				run.RetryOf = "run-100"
			}
		}

		if err := store.CreateRun(ctx, &run); err != nil {
			t.Fatal(err)
		}
	}

	if err := store.CreateRun(ctx, &database.ScheduleRun{ID: "other-server", ScheduleID: "other-schedule", ServerID: "other", CreatedAt: 300}); err != nil {
		t.Fatal(err)
	}

	first, err := store.ScheduleRuns(ctx, 1, true, "game", "", "")
	if err != nil || len(first) != 100 || first[0].ID != "run-101" || first[0].RetryOf != "run-100" {
		t.Fatal("schedule pagination lost scheduled attempts", len(first), err)
	}

	for _, run := range first {
		if run.ScheduleID == "" || run.Content != "" || run.ServerID != "game" {
			t.Fatal("schedule history leaked a direct operation, command, or server", run)
		}
	}

	last := first[len(first)-1]

	second, err := store.ScheduleRuns(ctx, 1, true, "game", fmt.Sprintf("%d:%s", last.CreatedAt, last.ID), "")
	if err != nil || len(second) != 2 || second[0].ID != "run-001" || second[1].ID != "run-000" {
		t.Fatal("schedule history cursor skipped attempts", second, err)
	}

	visible, err := store.ScheduleRuns(ctx, 999, false, "", "", "")
	if err != nil || len(visible) != 0 {
		t.Fatal("schedule history bypassed server visibility", visible, err)
	}

	operations, err := store.Runs(ctx, 1, true, "game", "")
	if err != nil || len(operations) != 100 || operations[0].ScheduleID != "" {
		t.Fatal("direct operation history was removed", len(operations), err)
	}

	for i := range 110 {
		if err := store.CreateRun(ctx, &database.ScheduleRun{ID: fmt.Sprintf("other-%03d", i), ScheduleID: "other-schedule", ServerID: "game", CreatedAt: int64(1000 + i)}); err != nil {
			t.Fatal(err)
		}
	}

	filtered, err := store.ScheduleRuns(ctx, 1, true, "game", "", "deleted-schedule")
	if err != nil || len(filtered) != 100 || filtered[0].ID != "run-101" {
		t.Fatal("schedule filter was applied after pagination", filtered, err)
	}

	filtered, err = store.ScheduleRuns(ctx, 1, true, "game", fmt.Sprintf("%d:%s", last.CreatedAt, last.ID), "deleted-schedule")
	if err != nil || len(filtered) != 2 || filtered[0].ID != "run-001" {
		t.Fatal("filtered history lost its cursor", filtered, err)
	}

	filtered, err = store.ScheduleRuns(ctx, 999, false, "", "", "deleted-schedule")
	if err != nil || len(filtered) != 0 {
		t.Fatal("schedule filter bypassed visibility", filtered, err)
	}
}

func TestHistoryPruningPreservesActiveAndRecentlyFinishedRecords(t *testing.T) {
	db, store := historyStore(t)
	ctx := context.Background()
	cutoff := time.Now().UTC().Truncate(time.Second).Add(-30 * 24 * time.Hour)

	old := cutoff.Add(-time.Hour).Unix()
	for _, state := range []string{"succeeded", "failed", "interrupted", "cancelled", "skipped", "queued", "dispatching", "running", "unknown"} {
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

	for _, outcome := range []string{"succeeded", "failed", "accepted", "interrupted", "cancelled", "skipped", "pending", "unknown"} {
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
	if err := db.ORM().Model(&database.ScheduleRun{}).Count(&count).Error; err != nil || count != 12 {
		t.Fatal("disabled retention changed records", count, err)
	}

	if more, err := store.PruneHistory(ctx, time.Time{}, cutoff); err != nil || more {
		t.Fatal(err)
	}

	if err := db.ORM().Model(&database.AuditEvent{}).Count(&count).Error; err != nil || count != 9 {
		t.Fatal("operation policy changed audit history", count, err)
	}

	runs, err := store.Runs(ctx, 1, true, "", "")
	if err != nil || len(runs) != 7 {
		t.Fatal("terminal pruning failed", len(runs), err)
	}

	for _, run := range runs {
		if run.ID == "succeeded" || run.ID == "failed" || run.ID == "interrupted" || run.ID == "cancelled" || run.ID == "skipped" {
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
