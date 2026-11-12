package repository_test

import (
	"errors"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

func TestRegisteredLabelsSurviveEventRetention(t *testing.T) {
	db, _ := resourceHistoryStore(t)
	store := repository.NewGameEventRepository(db.ORM())

	definition := model.GameRegistration{Revision: 1, EventFields: map[string]model.GameFieldLabels{"player.joined": {"player": "Player name"}}}
	if err := store.RegisterGame(t.Context(), "node", "game", definition); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	event := gameEvent(0, now)

	event.Type = "player.joined"
	if err := store.Append(t.Context(), "node", "game", event, now, time.Hour, 100); err != nil {
		t.Fatal(err)
	}

	if err := store.Prune(t.Context(), now.Add(time.Hour).Unix(), 100); err != nil {
		t.Fatal(err)
	}

	var labels []database.GameEventFieldLabel
	if err := db.ORM().Find(&labels).Error; err != nil || len(labels) != 1 || labels[0].Label != "Player name" {
		t.Fatal("registration metadata was pruned with event history", labels, err)
	}

	event.ID = "after-retention"
	if err := store.Append(t.Context(), "node", "game", event, now, time.Hour, 100); err != nil {
		t.Fatal(err)
	}

	page, err := store.List(t.Context(), "game", repository.GameEventQuery{Limit: 10})
	if err != nil || len(page.Items) != 1 || page.Items[0].FieldLabels["player"] != "Player name" {
		t.Fatal("label-free event lost registered label after retention", page, err)
	}
}

func TestGameRegistrationPersistenceRevisionAndLabels(t *testing.T) {
	db, _ := resourceHistoryStore(t)
	store := repository.NewGameEventRepository(db.ORM())

	definition := model.GameRegistration{Revision: 1, StateFields: model.GameFieldLabels{"map": "World"}, EventFields: map[string]model.GameFieldLabels{"player.joined": {"player": "Player name"}}}
	if err := store.RegisterGame(t.Context(), "wrong-node", "game", definition); !errors.Is(err, repository.ErrGameEventPlacement) {
		t.Fatal("unowned registration accepted", err)
	}

	for range 2 {
		if err := store.RegisterGame(t.Context(), "node", "game", definition); err != nil {
			t.Fatal(err)
		}
	}

	changed := definition

	changed.StateFields = model.GameFieldLabels{"map": "Changed"}
	if err := store.RegisterGame(t.Context(), "node", "game", changed); !errors.Is(err, repository.ErrGameRegistrationConflict) {
		t.Fatal("revision reused", err)
	}

	// Metadata survives without any events, including startup/hourly retention.
	if err := store.Prune(t.Context(), time.Now().Unix(), 100); err != nil {
		t.Fatal(err)
	}

	loaded, err := repository.NewGameEventRepository(db.ORM()).GameRegistration(t.Context(), "game")
	if err != nil || loaded == nil || loaded.StateFields["map"] != "World" {
		t.Fatal("registration lost", loaded, err)
	}

	var labels []database.GameEventFieldLabel
	if err := db.ORM().Find(&labels).Error; err != nil || len(labels) != 1 {
		t.Fatal("retention removed registered labels", labels, err)
	}

	now := time.Now().UTC()
	event := gameEvent(0, now)
	event.Type = "player.joined"
	// Even an old queued event cannot overwrite explicitly registered labels.
	event.FieldLabels = model.GameFieldLabels{"player": "Legacy label"}
	if err := store.Append(t.Context(), "node", "game", event, now, time.Hour, 100); err != nil {
		t.Fatal(err)
	}

	page, err := store.List(t.Context(), "game", repository.GameEventQuery{Limit: 10})
	if err != nil || len(page.Items) != 1 || page.Items[0].FieldLabels["player"] != "Player name" {
		t.Fatal("registration not applied", page, err)
	}

	activity := repository.NewServerActivityRepository(db.ORM())
	if _, err := activity.FilterChoices(t.Context(), "game", "game"); err != nil {
		t.Fatal(err)
	}

	definition.Revision = 2

	definition.EventFields["player.joined"]["player"] = "Member"
	if err := store.RegisterGame(t.Context(), "node", "game", definition); err != nil {
		t.Fatal(err)
	}

	page, err = store.List(t.Context(), "game", repository.GameEventQuery{Limit: 10})
	if err != nil || page.Items[0].FieldLabels["player"] != "Member" {
		t.Fatal("history did not use updated registration", page, err)
	}

	choices, err := activity.FilterChoices(t.Context(), "game", "game")
	if err != nil || choices.Game.FieldLabels["player.joined"]["player"] != "Member" {
		t.Fatal("registration did not invalidate cached labels", choices, err)
	}

	definition.Revision = 1
	if err := store.RegisterGame(t.Context(), "node", "game", definition); !errors.Is(err, repository.ErrGameRegistrationConflict) {
		t.Fatal("stale revision accepted", err)
	}

	definition.Revision = 3

	definition.EventFields = nil
	if err := store.RegisterGame(t.Context(), "node", "game", definition); err != nil {
		t.Fatal(err)
	}

	page, err = store.List(t.Context(), "game", repository.GameEventQuery{Limit: 10})
	if err != nil || len(page.Items[0].FieldLabels) != 0 {
		t.Fatal("removed labels still present", page, err)
	}

	if err := db.ORM().Delete(&database.Server{}, "id = ?", "game").Error; err != nil {
		t.Fatal(err)
	}

	if loaded, err := store.GameRegistration(t.Context(), "game"); err != nil || loaded != nil {
		t.Fatal("deleted server retained registration", loaded, err)
	}
}
