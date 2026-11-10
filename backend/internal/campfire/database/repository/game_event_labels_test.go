package repository_test

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

func TestGameEventLabelsUseLatestDefinitionsWithinEachType(t *testing.T) {
	db, _ := resourceHistoryStore(t)
	store := repository.NewGameEventRepository(db.ORM())
	now := time.Now().UTC()
	appendEvent := func(index int, eventType, label string) model.GameEvent {
		t.Helper()

		event := gameEvent(index, now)

		event.Type = eventType
		if label != "" {
			event.FieldLabels = model.GameFieldLabels{"player": label}
		}

		if err := store.Append(t.Context(), "node", "game", event, now, time.Hour, 100); err != nil {
			t.Fatal(err)
		}

		return event
	}
	original := appendEvent(0, "player.joined", "Player name")
	appendEvent(1, "player.joined", "New player label")
	appendEvent(2, "player.left", "Departing player")
	appendEvent(3, "player.joined", "")
	// Replaying an old event must not revert newer dictionary metadata.
	if err := store.Append(t.Context(), "node", "game", original, now, time.Hour, 100); err != nil {
		t.Fatal(err)
	}

	page, err := store.List(t.Context(), "game", repository.GameEventQuery{Limit: 10})
	if err != nil || len(page.Items) != 4 {
		t.Fatal(page, err)
	}

	for _, event := range page.Items {
		want := "New player label"
		if event.Type == "player.left" {
			want = "Departing player"
		}

		if event.FieldLabels["player"] != want {
			t.Fatal("history did not use current, type-scoped label", event)
		}
	}

	options, err := store.FilterOptions(t.Context(), "game")
	if err != nil || options.FieldLabels["player.joined"]["player"] != "New player label" || options.FieldLabels["player.left"]["player"] != "Departing player" || !reflect.DeepEqual(options.Fields["player"], []string{"Alex"}) {
		t.Fatal("dictionary changed filter identity or values", options, err)
	}

	if db.ORM().Migrator().HasColumn(&database.GameEvent{}, "field_labels") {
		t.Fatal("labels must not be stored on each event")
	}
}

func TestGameEventLabelRetentionRemovesOnlyUnreferencedDefinitions(t *testing.T) {
	db, _ := resourceHistoryStore(t)
	store := repository.NewGameEventRepository(db.ORM())

	now := time.Now().UTC()
	for i, fields := range []map[string]any{{"retired": true, "shared": true}, {"shared": false}, {"other": true}} {
		event := gameEvent(i, now)
		event.Type, event.Data = "game.status", fields

		event.FieldLabels = model.GameFieldLabels{}
		for key := range fields {
			event.FieldLabels[key] = "Label " + key
		}

		if err := store.Append(t.Context(), "node", "game", event, now.Add(time.Duration(i)*time.Minute), time.Hour, 100); err != nil {
			t.Fatal(err)
		}
	}

	if err := store.Prune(t.Context(), now.Add(time.Minute).Unix(), 100); err != nil {
		t.Fatal(err)
	}

	var rows []database.GameEventFieldLabel
	if err := db.ORM().Order("field_key").Find(&rows).Error; err != nil || len(rows) != 2 || rows[0].FieldKey != "other" || rows[1].FieldKey != "shared" {
		t.Fatal("expired field label was retained or shared label lost", rows, err)
	}

	extra := gameEvent(3, now)

	extra.Type, extra.Data = "game.status", map[string]any{"other": true}
	if err := store.Append(t.Context(), "node", "game", extra, now.Add(2*time.Minute), time.Hour, 1); err != nil {
		t.Fatal(err)
	}

	options, err := store.FilterOptions(t.Context(), "game")
	if err != nil || len(options.Fields) != 1 || options.FieldLabels["game.status"]["shared"] != "" {
		t.Fatal("ingestion eviction left retired fields in discovery", options, err)
	}

	if err := store.Prune(t.Context(), 0, 1); err != nil {
		t.Fatal(err)
	}

	rows = nil
	if err := db.ORM().Find(&rows).Error; err != nil || len(rows) != 1 || rows[0].FieldKey != "other" {
		t.Fatal("count retention left stale labels", rows, err)
	}

	if err := store.Prune(t.Context(), now.Add(3*time.Minute).Unix(), 100); err != nil {
		t.Fatal(err)
	}

	var count int64
	if err := db.ORM().Model(&database.GameEventFieldLabel{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("quiet server retained orphan labels", count, err)
	}
}

func TestGameEventLabelDictionaryUpgradePreservesExistingHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upgrade.db")

	db, err := database.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	for _, row := range []any{
		&database.Spark{ID: "node", Name: "Node"},
		&database.Ember{ID: "ember", Name: "Game", Definition: model.Ember{Schema: 1}},
		&database.Server{ID: "game", Name: "Game", SparkID: "node", EmberID: "ember", Image: "image", GameIntegration: true},
		&database.GameEvent{ServerID: "game", SparkID: "node", EventID: "existing", Type: "player.joined", Title: "Existing event", OccurredAt: now, ReceivedAt: now.Unix(), Data: map[string]any{"player": "Alex"}},
	} {
		if err := db.ORM().Omit("Spark", "Ember", "Owner", "Server").Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}

	// Model an existing release database without the new dictionary table.
	if err := db.ORM().Migrator().DropTable(&database.GameEventFieldLabel{}); err != nil {
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

	store := repository.NewGameEventRepository(db.ORM())
	event := gameEvent(1, now)

	event.Type, event.FieldLabels = "player.joined", model.GameFieldLabels{"player": "Player name"}
	if err := store.Append(t.Context(), "node", "game", event, now, time.Hour, 100); err != nil {
		t.Fatal(err)
	}

	page, err := store.List(t.Context(), "game", repository.GameEventQuery{Limit: 10})
	if err != nil || len(page.Items) != 2 || page.Items[1].EventID != "existing" || page.Items[1].FieldLabels["player"] != "Player name" {
		t.Fatal("dictionary migration changed history or failed label discovery", page, err)
	}
}
