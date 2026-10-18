package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"campfire/internal/shared/model"
)

type memoryEmberUpdates struct{ items map[string]model.EmberUpdate }

func (s *memoryEmberUpdates) List(context.Context) ([]model.EmberUpdate, error) {
	items := make([]model.EmberUpdate, 0, len(s.items))
	for _, item := range s.items {
		items = append(items, item)
	}

	return items, nil
}

func (s *memoryEmberUpdates) Save(_ context.Context, item model.EmberUpdate) error {
	s.items[item.EmberID] = item
	return nil
}

func TestEmberUpdateChecksCompareDefinitionsAndRespectDailyInterval(t *testing.T) {
	store := &importMemoryStore{}
	embers := NewEmberService(store)

	imported, err := embers.ImportDefinition(t.Context(), eggFixture(t, "native.json"), ImportOptions{SourceURL: "https://example.com/game.json"})
	if err != nil {
		t.Fatal(err)
	}

	checks := &memoryEmberUpdates{items: map[string]model.EmberUpdate{}}
	s := NewEmberUpdateService(embers, checks)
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	remote := imported
	remote.ID = "different-import-id"
	remote.UpdateURL = "https://example.com/new-location.json"
	fetches := 0
	fail := false

	s.fetch = func(_ context.Context, address string, _ ImportOptions) (EmberPreview, error) {
		fetches++

		if address != imported.UpdateURL {
			t.Fatal("wrong update URL", address)
		}

		if fail {
			return EmberPreview{}, errors.New("source unreachable")
		}

		data, err := json.MarshalIndent(remote, "", "  ")
		if err != nil {
			t.Fatal(err)
		}

		return PreviewEmber(data, ImportOptions{}), nil
	}
	if err := s.Sweep(t.Context()); err != nil {
		t.Fatal(err)
	}

	first := checks.items[imported.ID]
	if fetches != 1 || first.Available || first.Error != "" || first.CheckedAt != now.Unix() {
		t.Fatal("unchanged revision detected as update", first, fetches)
	}

	now = now.Add(23 * time.Hour)

	if err := s.Sweep(t.Context()); err != nil || fetches != 1 {
		t.Fatal("checked before daily interval", err, fetches)
	}

	remote.Installer = &model.Installer{Image: "alpine:3.22", Entrypoint: "sh", Script: "echo new version"}
	now = now.Add(time.Hour)

	if err := s.Sweep(t.Context()); err != nil {
		t.Fatal(err)
	}

	if item := checks.items[imported.ID]; fetches != 2 || !item.Available || item.Error != "" || item.CheckedAt != now.Unix() {
		t.Fatal("changed revision was not detected", item, fetches)
	}

	item, preview, err := s.Check(t.Context(), imported.ID)
	if err != nil || !item.Available || preview.Ember == nil || preview.Ember.Installer.Script != "echo new version" || len(store.saved) != 1 {
		t.Fatal("manual download changed imports or lost candidate", item, preview, err)
	}

	fail = true

	item, _, err = s.Check(t.Context(), imported.ID)
	if err != nil || item.Error == "" || item.Available {
		t.Fatal("failed fetch reported up to date", item, err)
	}
	// A restart uses persisted check times, including failed checks.
	restarted := NewEmberUpdateService(embers, checks)
	restarted.now = s.now
	restarted.fetch = s.fetch

	before := fetches
	if err := restarted.Sweep(t.Context()); err != nil || fetches != before {
		t.Fatal("restart ignored saved last-check time", err, fetches)
	}

	if store.saved[0].Installer != imported.Installer {
		t.Fatal("check modified immutable definition")
	}
}

func TestEmberUpdateInvalidSourceAndCanceledQueue(t *testing.T) {
	store := &importMemoryStore{}
	embers := NewEmberService(store)

	imported, err := embers.ImportDefinition(t.Context(), eggFixture(t, "native.json"), ImportOptions{SourceURL: "https://example.com/game.json"})
	if err != nil {
		t.Fatal(err)
	}

	checks := &memoryEmberUpdates{items: map[string]model.EmberUpdate{}}
	s := NewEmberUpdateService(embers, checks)
	s.fetch = func(context.Context, string, ImportOptions) (EmberPreview, error) {
		return PreviewEmber([]byte("not a definition"), ImportOptions{}), nil
	}

	item, _, err := s.Check(t.Context(), imported.ID)
	if err != nil || item.Error == "" || item.Available {
		t.Fatal("invalid remote definition accepted", item, err)
	}

	s.gate <- struct{}{}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, _, err := s.Check(ctx, imported.ID); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled request waited for check queue", err)
	}

	<-s.gate
}
