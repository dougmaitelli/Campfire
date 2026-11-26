package repository_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

func retainedCrash(index int, at time.Time) model.CrashRecord {
	return model.CrashRecord{ID: fmt.Sprintf("%064x", index), ContainerID: strings.Repeat("a", 64), ExitedAt: at.Unix(), ExitCode: 137, Reason: model.CrashReasonOutOfMemory, Console: "private-crash-evidence"}
}

func TestCrashRetentionPlacementReplayAndActivityEvidence(t *testing.T) {
	db, _ := integrationEventStore(t)
	store := repository.NewCrashRepository(db.ORM())
	activity := repository.NewServerActivityRepository(db.ORM())

	record := retainedCrash(1, time.Now())
	if err := store.Observe(t.Context(), "wrong-node", "first", []model.CrashRecord{record}); err == nil {
		t.Fatal("accepted wrong placement")
	}

	for range 2 {
		if err := store.Observe(t.Context(), "node", "first", []model.CrashRecord{record}); err != nil {
			t.Fatal(err)
		}
	}

	page, err := activity.List(t.Context(), "first", repository.ServerActivityAccess{}, repository.ServerActivityFilter{Kind: "alert"})
	if err != nil || len(page.Items) != 1 || page.Items[0].Kind != "alert" || page.Items[0].Title != record.Title() || page.Items[0].Key != "crash:"+record.ID {
		t.Fatal(page, err)
	}

	for _, console := range []bool{false, true} {
		details, err := activity.Details(t.Context(), "first", page.Items[0].Key, repository.ServerActivityAccess{Console: console})
		if err != nil || details.Crash == nil || details.Crash.ExitCode != 137 {
			t.Fatal(details, err)
		}

		data, err := json.Marshal(details)
		if err != nil || strings.Contains(string(data), record.Console) != console {
			t.Fatal("console evidence authorization failed", string(data), err)
		}
	}

	if _, err := activity.Details(t.Context(), "second", page.Items[0].Key, repository.ServerActivityAccess{Console: true}); err == nil {
		t.Fatal("cross-server evidence access")
	}

	matches, err := activity.List(t.Context(), "first", repository.ServerActivityAccess{Console: true}, repository.ServerActivityFilter{Search: record.Console})
	if err != nil || len(matches.Items) != 0 {
		t.Fatal("console became searchable", matches, err)
	}

	for index := 2; index <= model.CrashHistoryLimit+5; index++ {
		next := retainedCrash(index, time.Now())
		if err := store.Observe(t.Context(), "node", "first", []model.CrashRecord{next}); err != nil {
			t.Fatal(err)
		}
	}

	var count int64
	if err := db.ORM().Model(&database.ServerCrash{}).Where("server_id = ?", "first").Count(&count).Error; err != nil || count != model.CrashHistoryLimit {
		t.Fatal(count, err)
	}

	old := retainedCrash(999, time.Now().Add(-model.CrashRetention-time.Hour))
	if err := store.Observe(t.Context(), "node", "first", []model.CrashRecord{old}); err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Where("id = ?", old.ID).Model(&database.ServerCrash{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("resurrected expired evidence", count, err)
	}

	if err := db.ORM().Model(&database.ServerCrash{}).Where("server_id = ?", "first").Update("exited_at", time.Now().Add(-model.CrashRetention-time.Hour).Unix()).Error; err != nil {
		t.Fatal(err)
	}

	if err := store.Prune(t.Context()); err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Model(&database.ServerCrash{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("offline retention failed", count, err)
	}
}

func TestCrashObservationBoundsAreValidatedBeforeAnyInsert(t *testing.T) {
	db, _ := integrationEventStore(t)
	store := repository.NewCrashRepository(db.ORM())
	valid := retainedCrash(1, time.Now())
	invalid := valid
	invalid.ID = strings.Repeat("b", 64)

	invalid.Console = strings.Repeat("x", model.CrashConsoleBytes+1)
	if err := store.Observe(t.Context(), "node", "first", []model.CrashRecord{valid, invalid}); err == nil {
		t.Fatal("accepted oversized evidence")
	}

	if err := store.Observe(t.Context(), "node", "first", make([]model.CrashRecord, model.CrashReplayLimit+1)); err == nil {
		t.Fatal("accepted oversized replay")
	}

	var count int64
	if err := db.ORM().Model(&database.ServerCrash{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("partial insert", count, err)
	}
}
