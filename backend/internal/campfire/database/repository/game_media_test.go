package repository_test

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

func TestDisabledGameIntegrationRejectsEventsAndMediaRetries(t *testing.T) {
	db, _ := resourceHistoryStore(t)
	events := repository.NewGameEventRepository(db.ORM())
	mediaStore := repository.NewGameMediaRepository(db.ORM())
	now := time.Now()

	event := gameEvent(0, now)
	if err := events.Append(t.Context(), "node", "game", event, now, time.Hour, 10); err != nil {
		t.Fatal(err)
	}

	media := model.GameMedia{ID: "shot", Name: "world.png", ContentType: "image/png", Size: 5}
	if err := mediaStore.Append(t.Context(), "node", "game", event.ID, media, []byte("image")); err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Model(&database.Server{}).Where("id = ?", "game").Update("game_integration", false).Error; err != nil {
		t.Fatal(err)
	}

	if err := events.Append(t.Context(), "node", "game", event, now, time.Hour, 10); !errors.Is(err, repository.ErrGameEventPlacement) {
		t.Fatal("disabled integration accepted an event retry", err)
	}

	if err := mediaStore.Append(t.Context(), "node", "game", event.ID, media, []byte("image")); !errors.Is(err, model.ErrGameMediaPlacement) {
		t.Fatal("disabled integration accepted a media retry", err)
	}
}

func TestGameMediaIdentityQuotasAndCascadingRetention(t *testing.T) {
	db, _ := resourceHistoryStore(t)
	events := repository.NewGameEventRepository(db.ORM())
	store := repository.NewGameMediaRepository(db.ORM())

	now := time.Now()
	if err := events.Append(t.Context(), "node", "game", gameEvent(0, now), now, time.Hour, 10); err != nil {
		t.Fatal(err)
	}

	media := model.GameMedia{ID: "shot", Name: "world.png", ContentType: "image/png", Size: 5}

	var workers sync.WaitGroup
	for range 8 {
		workers.Go(func() {
			if err := store.Append(t.Context(), "node", "game", "event-0", media, []byte("image")); err != nil {
				t.Error(err)
			}
		})
	}

	workers.Wait()

	page, err := events.List(t.Context(), "game", repository.GameEventQuery{Limit: 10})
	if err != nil || len(page.Items[0].Attachments) != 1 || page.Items[0].Attachments[0].Data != nil {
		t.Fatal("list must include metadata without blobs", page, err)
	}

	id := page.Items[0].ID

	if err := store.Append(t.Context(), "node", "game", "event-0", media, []byte("other")); !errors.Is(err, model.ErrGameMediaConflict) {
		t.Fatal("conflicting retry accepted", err)
	}

	if err := store.Append(t.Context(), "other", "game", "event-0", media, []byte("image")); !errors.Is(err, model.ErrGameMediaPlacement) {
		t.Fatal("foreign source accepted", err)
	}

	if err := store.Append(t.Context(), "node", "game", "missing", media, []byte("image")); !errors.Is(err, model.ErrGameMediaEventMissing) {
		t.Fatal("missing event accepted", err)
	}

	for i := 1; i < model.GameMediaEventLimit; i++ {
		media.ID = fmt.Sprintf("shot-%d", i)
		if err := store.Append(t.Context(), "node", "game", "event-0", media, []byte("image")); err != nil {
			t.Fatal(err)
		}
	}

	media.ID = "overflow"
	if err := store.Append(t.Context(), "node", "game", "event-0", media, []byte("image")); !errors.Is(err, model.ErrGameMediaLimit) {
		t.Fatal("attachment cap ignored", err)
	}

	if _, err := store.Get(t.Context(), "other", id, "shot"); !errors.Is(err, model.ErrGameMediaNotFound) {
		t.Fatal("download escaped server scope", err)
	}

	if err := events.Prune(t.Context(), now.Add(time.Second).Unix(), 10); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Get(t.Context(), "game", id, "shot"); !errors.Is(err, model.ErrGameMediaNotFound) {
		t.Fatal("event retention left orphan bytes", err)
	}

	if err := events.Append(t.Context(), "node", "game", gameEvent(1, now), now, time.Hour, 10); err != nil {
		t.Fatal(err)
	}

	if err := store.Append(t.Context(), "node", "game", "event-1", media, []byte("image")); err != nil {
		t.Fatal("retention did not release quota", err)
	}

	if err := db.ORM().Where("id = ?", "game").Delete(&database.Server{}).Error; err != nil {
		t.Fatal(err)
	}

	var count int64
	if err := db.ORM().Model(&database.GameMedia{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("server deletion left media", count, err)
	}
}

func TestGameMediaStorageBudgets(t *testing.T) {
	db, _ := resourceHistoryStore(t)
	events := repository.NewGameEventRepository(db.ORM())
	store := repository.NewGameMediaRepository(db.ORM())

	now := time.Now()
	for i := range 2 {
		if err := events.Append(t.Context(), "node", "game", gameEvent(i, now), now, time.Hour, 10); err != nil {
			t.Fatal(err)
		}
	}

	media := model.GameMedia{ID: "shot", Name: "world.png", ContentType: "image/png", Size: 5}
	if err := store.Append(t.Context(), "node", "game", "event-0", media, []byte("image")); err != nil {
		t.Fatal(err)
	}
	// Account for a full store without allocating hundreds of MiB in a test.
	if err := db.ORM().Model(&database.GameMedia{}).Where("server_id = ?", "game").Update("size", model.GameMediaServerLimit).Error; err != nil {
		t.Fatal(err)
	}

	if err := store.Append(t.Context(), "node", "game", "event-1", media, []byte("image")); !errors.Is(err, model.ErrGameMediaLimit) {
		t.Fatal("server storage cap ignored", err)
	}

	if err := db.ORM().Model(&database.GameMedia{}).Where("server_id = ?", "game").Updates(map[string]any{"server_id": "another-server", "size": model.GameMediaTotalLimit}).Error; err != nil {
		t.Fatal(err)
	}

	if err := store.Append(t.Context(), "node", "game", "event-1", media, []byte("image")); !errors.Is(err, model.ErrGameMediaLimit) {
		t.Fatal("global storage cap ignored", err)
	}

	if err := db.ORM().Model(&database.Spark{}).Where("id = ?", "node").Update("disabled", true).Error; err != nil {
		t.Fatal(err)
	}

	if err := store.Append(t.Context(), "node", "game", "event-0", media, []byte("image")); !errors.Is(err, model.ErrGameMediaPlacement) {
		t.Fatal("disabled Spark accepted a media retry", err)
	}
}
