package repository_test

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

func gameEvent(index int, now time.Time) model.GameEvent {
	kind := "game.announcement"
	if index%2 == 0 {
		kind = "player.joined"
	}

	return model.GameEvent{ID: fmt.Sprintf("event-%d", index), Type: kind, Title: fmt.Sprintf("Event %d", index), OccurredAt: now.Add(time.Duration(index) * time.Minute), Data: map[string]any{"player": "Alex"}}
}

func TestGameEventDeduplicationPaginationRetentionAndPlacement(t *testing.T) {
	db, _ := resourceHistoryStore(t)
	store := repository.NewGameEventRepository(db.ORM())
	now := time.Now().UTC().Truncate(time.Second)

	var workers sync.WaitGroup
	for range 10 {
		workers.Go(func() {
			if err := store.Append(t.Context(), "node", "game", gameEvent(0, now), now, 30*24*time.Hour, 3); err != nil {
				t.Error(err)
			}
		})
	}

	workers.Wait()

	for i := 1; i < 5; i++ {
		if err := store.Append(t.Context(), "node", "game", gameEvent(i, now), now, 30*24*time.Hour, 3); err != nil {
			t.Fatal(err)
		}
	}

	page, err := store.List(t.Context(), "game", repository.GameEventQuery{Limit: 2})
	if err != nil || len(page.Items) != 2 || page.NextBefore == 0 || page.Items[0].EventID != "event-4" {
		t.Fatal("incorrect first page", page, err)
	}

	second, err := store.List(t.Context(), "game", repository.GameEventQuery{Before: page.NextBefore, Limit: 2})
	if err != nil || len(second.Items) != 1 || second.Items[0].EventID != "event-2" || second.NextBefore != 0 {
		t.Fatal("retention or cursor failed", second, err)
	}

	filtered, err := store.List(t.Context(), "game", repository.GameEventQuery{Type: "player.joined", From: now.Add(3 * time.Minute).Unix(), To: now.Add(5 * time.Minute).Unix(), Limit: 10})
	if err != nil || len(filtered.Items) != 1 || filtered.Items[0].EventID != "event-4" {
		t.Fatal("type/time filter failed", filtered, err)
	}

	duplicate := gameEvent(4, now)

	duplicate.Title = "Must not replace original"
	if err := store.Append(t.Context(), "node", "game", duplicate, now, 30*24*time.Hour, 3); err != nil {
		t.Fatal(err)
	}

	page, err = store.List(t.Context(), "game", repository.GameEventQuery{Limit: 10})
	if err != nil || len(page.Items) != 3 || page.Items[0].Title != "Event 4" {
		t.Fatal("duplicate overwrote retained event", page, err)
	}

	if err := store.Append(t.Context(), "other", "game", gameEvent(99, now), now, 30*24*time.Hour, 3); !errors.Is(err, repository.ErrGameEventPlacement) {
		t.Fatal("wrong Spark accepted", err)
	}

	if err := store.Append(t.Context(), "node", "missing", gameEvent(99, now), now, 30*24*time.Hour, 3); !errors.Is(err, repository.ErrGameEventPlacement) {
		t.Fatal("missing server accepted", err)
	}

	if err := store.Prune(t.Context(), now.Add(time.Second).Unix(), 3); err != nil {
		t.Fatal(err)
	}

	page, err = store.List(t.Context(), "game", repository.GameEventQuery{Limit: 10})
	if err != nil || len(page.Items) != 0 {
		t.Fatal("quiet server retention failed", page, err)
	}
}

func TestGameEventsSurviveDatabaseReopenAndCascadeWithServer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.db")

	db, err := database.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}

	for _, row := range []any{
		&database.Spark{ID: "node", Name: "Node"},
		&database.Ember{ID: "ember", Name: "Game", Definition: model.Ember{Schema: 1}},
		&database.Server{ID: "game", Name: "Game", SparkID: "node", EmberID: "ember", Image: "image", Variables: map[string]string{}, Ports: []model.ServerPort{}},
	} {
		if err := db.ORM().Omit("Spark", "Ember", "Owner").Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}

	now := time.Now()

	store := repository.NewGameEventRepository(db.ORM())
	if err := store.Append(t.Context(), "node", "game", gameEvent(0, now), now, 24*time.Hour, 10); err != nil {
		t.Fatal(err)
	}

	media := model.GameMedia{ID: "shot", Name: "world.png", ContentType: "image/png", Size: 5}
	if err := repository.NewGameMediaRepository(db.ORM()).Append(t.Context(), "node", "game", "event-0", media, []byte("image")); err != nil {
		t.Fatal(err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = database.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	store = repository.NewGameEventRepository(db.ORM())
	if err := store.Append(t.Context(), "node", "game", gameEvent(0, now), now, 24*time.Hour, 10); err != nil {
		t.Fatal(err)
	}

	page, err := store.List(t.Context(), "game", repository.GameEventQuery{Limit: 10})
	if err != nil || len(page.Items) != 1 {
		t.Fatal("restart lost event or deduplication", page, err)
	}

	attachment, err := repository.NewGameMediaRepository(db.ORM()).Get(t.Context(), "game", page.Items[0].ID, "shot")
	if err != nil || string(attachment.Data) != "image" || len(page.Items[0].Attachments) != 1 {
		t.Fatal("restart lost attachment bytes or metadata", attachment, err)
	}

	if err := db.ORM().Delete(&database.Server{ID: "game"}).Error; err != nil {
		t.Fatal(err)
	}

	page, err = store.List(t.Context(), "game", repository.GameEventQuery{Limit: 10})
	if err != nil || len(page.Items) != 0 {
		t.Fatal("server deletion retained orphan events", page, err)
	}
}
