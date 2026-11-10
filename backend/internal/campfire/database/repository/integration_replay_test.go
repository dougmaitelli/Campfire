package repository_test

import (
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/integrations"
	"campfire/internal/shared/model"
)

func TestIntegrationSourceReplayAfterDatabaseReopen(t *testing.T) {
	db, _, source := integrationStore(t)
	if err := db.ORM().Model(&database.Server{}).Where("id = ?", "first").Update("game_integration", true).Error; err != nil {
		t.Fatal(err)
	}

	store, now := repository.NewGameEventRepository(db.ORM()), time.Now()

	event := model.GameEvent{ID: "restart-safe", Type: "player.joined", Title: "Joined", OccurredAt: now, Data: map[string]any{"state": "ready"}}
	if err := store.Append(t.Context(), "node", "first", event, now, 24*time.Hour, 100); err != nil {
		t.Fatal(err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := database.Open(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = reopened.Close() })

	if err := repository.NewGameEventRepository(reopened.ORM()).Append(t.Context(), "node", "first", event, now, 24*time.Hour, 100); err != nil {
		t.Fatal(err)
	}

	if messages := outgoingEvents(t, reopened); len(messages) != 1 || messages[0].EventID != integrations.GameEvent("first", event).ID {
		t.Fatal("reopen lost source deduplication or pending routing", messages)
	}
}
