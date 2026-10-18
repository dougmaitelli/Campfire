package repository_test

import (
	"path/filepath"
	"testing"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

func TestEmberUpdateChecksPersistAndCascadeWithoutChangingDefinitions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.db")

	db, err := database.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}

	embers := repository.NewEmberRepository(db.ORM())

	ember := model.Ember{ID: "original", Name: "Game", UpdateURL: "https://example.com/game.json"}
	if err := embers.Create(t.Context(), ember); err != nil {
		t.Fatal(err)
	}

	checks := repository.NewEmberUpdateRepository(db.ORM())

	item := model.EmberUpdate{EmberID: ember.ID, CheckedAt: 100, Available: true}
	if err := checks.Save(t.Context(), item); err != nil {
		t.Fatal(err)
	}

	item.CheckedAt = 200
	item.Available = false

	item.Error = "source unreachable"
	if err := checks.Save(t.Context(), item); err != nil {
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

	checks = repository.NewEmberUpdateRepository(db.ORM())

	items, err := checks.List(t.Context())
	if err != nil || len(items) != 1 || items[0] != item {
		t.Fatal("last check did not survive restart", items, err)
	}

	embers = repository.NewEmberRepository(db.ORM())

	saved, err := embers.Get(t.Context(), ember.ID)
	if err != nil || saved.ID != ember.ID || saved.Name != ember.Name || saved.UpdateURL != ember.UpdateURL {
		t.Fatal("check modified imported revision", saved, err)
	}

	if err := embers.Delete(t.Context(), ember.ID); err != nil {
		t.Fatal(err)
	}

	items, err = checks.List(t.Context())
	if err != nil || len(items) != 0 {
		t.Fatal("deleting Ember retained orphan check", items, err)
	}
}
