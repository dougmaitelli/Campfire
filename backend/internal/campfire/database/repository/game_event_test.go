package repository_test

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
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
		&database.Server{ID: "game", Name: "Game", SparkID: "node", EmberID: "ember", Image: "image", GameIntegration: true, Variables: map[string]string{}, Ports: []model.ServerPort{}},
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

func TestGameEventSearchAndFieldFiltersBeforePagination(t *testing.T) {
	db, _ := resourceHistoryStore(t)
	store := repository.NewGameEventRepository(db.ORM())
	now := time.Now().UTC().Truncate(time.Second)
	escaped := "A_%!<&\"\\世界"
	data := []map[string]any{
		{"player": "Alex", "state": "warning", "score": 1, "online": false, "player.name": escaped, "empty": ""},
		{"player": "Sam", "state": "warning", "score": 10},
		{"player": "Alex", "state": "ok", "score": 1.5},
		{"message": "Hello world!"},
		{"online": "false"},
		{},
		{"player": "Alex", "state": "warning", "score": 10},
		{"player.name": escaped},
	}

	for i, fields := range data {
		event := gameEvent(i, now)
		event.Data = fields

		if i == 3 {
			event.Title = "Alex announcement"
		}

		if err := store.Append(t.Context(), "node", "game", event, now, 24*time.Hour, 100); err != nil {
			t.Fatal(err)
		}
	}

	for _, test := range []struct {
		name  string
		query repository.GameEventQuery
		want  []string
	}{
		{"title and values", repository.GameEventQuery{Search: "alex"}, []string{"event-6", "event-3", "event-2", "event-0"}},
		{"literal wildcards", repository.GameEventQuery{Search: "A_%!"}, []string{"event-7", "event-0"}},
		{"JSON escapes", repository.GameEventQuery{Search: "<&\"\\世界"}, []string{"event-7", "event-0"}},
		{"combined fields", repository.GameEventQuery{Fields: map[string]string{"player": "alex", "state": "WARNING"}}, []string{"event-6", "event-0"}},
		{"field names are literal", repository.GameEventQuery{Fields: map[string]string{"player.name": escaped}}, []string{"event-7", "event-0"}},
		{"field existence", repository.GameEventQuery{Fields: map[string]string{"empty": ""}}, []string{"event-0"}},
		{"boolean false", repository.GameEventQuery{Fields: map[string]string{"online": "false"}}, []string{"event-4", "event-0"}},
		{"numeric boundaries", repository.GameEventQuery{Fields: map[string]string{"score": "1"}}, []string{"event-0"}},
		{"partial field value does not match", repository.GameEventQuery{Fields: map[string]string{"state": "warn"}}, []string{}},
		{"wildcards are literal in values", repository.GameEventQuery{Fields: map[string]string{"state": "warn%"}}, []string{}},
		{"value text cannot match another key", repository.GameEventQuery{Fields: map[string]string{"state": "Alex"}}, []string{}},
		{"search and fields intersect", repository.GameEventQuery{Search: "Sam", Fields: map[string]string{"state": "warning"}}, []string{"event-1"}},
		{"existing type filter combines", repository.GameEventQuery{Type: "player.joined", Fields: map[string]string{"state": "warning"}}, []string{"event-6", "event-0"}},
		{"time range combines", repository.GameEventQuery{From: now.Add(time.Minute).Unix(), To: now.Add(7 * time.Minute).Unix(), Fields: map[string]string{"state": "warning"}}, []string{"event-6", "event-1"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.query.Limit = 100

			page, err := store.List(t.Context(), "game", test.query)
			if err != nil {
				t.Fatal(err)
			}

			ids := make([]string, 0, len(page.Items))
			for _, item := range page.Items {
				ids = append(ids, item.EventID)
			}

			if !reflect.DeepEqual(ids, test.want) {
				t.Fatal("filter returned incorrect events", ids, test.want)
			}
		})
	}

	query := repository.GameEventQuery{Fields: map[string]string{"player": "Alex", "state": "warning"}, Limit: 1}
	options, err := store.FilterOptions(t.Context(), "game")

	want := repository.GameEventFilterOptions{
		Types: []string{"game.announcement", "player.joined"},
		Fields: map[string][]string{
			"player": {"Alex", "Sam"}, "state": {"ok", "warning"},
			"score": {"1", "1.5", "10"}, "online": {"false"},
			"player.name": {escaped}, "empty": {}, "message": {"Hello world!"},
		},
	}

	if err != nil || !reflect.DeepEqual(options, want) {
		t.Fatal("filter options did not cover retained history", options, err)
	}

	emptyOptions, err := store.FilterOptions(t.Context(), "missing")
	if err != nil || len(emptyOptions.Types) != 0 || len(emptyOptions.Fields) != 0 {
		t.Fatal("filter options crossed server boundaries", emptyOptions, err)
	}

	page, err := store.List(t.Context(), "game", query)
	if err != nil || len(page.Items) != 1 || page.Items[0].EventID != "event-6" || page.NextBefore == 0 {
		t.Fatal("first filtered page was incorrect", page, err)
	}

	query.Before = page.NextBefore

	page, err = store.List(t.Context(), "game", query)
	if err != nil || len(page.Items) != 1 || page.Items[0].EventID != "event-0" || page.NextBefore != 0 {
		t.Fatal("filtering skipped older matches", page, err)
	}

	page, err = store.List(t.Context(), "missing", repository.GameEventQuery{Search: "Alex", Fields: query.Fields, Limit: 100})
	if err != nil || len(page.Items) != 0 {
		t.Fatal("filters crossed server boundaries", page, err)
	}
}
