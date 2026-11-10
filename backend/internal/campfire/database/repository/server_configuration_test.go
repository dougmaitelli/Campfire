package repository_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"campfire/internal/campfire/database/repository"
)

func TestConfigurationAcknowledgementDoesNotApproveAConcurrentServerEdit(t *testing.T) {
	db, _ := resourceHistoryStore(t)
	if err := db.ORM().Exec("UPDATE servers SET created_at = ? WHERE id = ?", time.Now().UTC().Format(time.RFC3339Nano), "game").Error; err != nil {
		t.Fatal(err)
	}

	store := repository.NewServerRepository(db.ORM())

	before, err := store.Get(t.Context(), "game")
	if err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Exec("UPDATE servers SET memory_mb = memory_mb + 128 WHERE id = ?", before.ID).Error; err != nil {
		t.Fatal(err)
	}

	if err := store.AcknowledgeConfiguration(t.Context(), before, strings.Repeat("a", 64)); !errors.Is(err, repository.ErrServerConflict) {
		t.Fatal("stale delivery acknowledged newer server values", err)
	}

	after, err := store.Get(t.Context(), before.ID)
	if err != nil || after.EmberHash != before.EmberHash || after.MemoryMB != before.MemoryMB+128 {
		t.Fatal("stale acknowledgement changed server values", after, err)
	}

	if err := store.AcknowledgeConfiguration(t.Context(), after, strings.Repeat("b", 64)); err != nil {
		t.Fatal(err)
	}
}
