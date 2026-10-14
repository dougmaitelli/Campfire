package repository_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"campfire/internal/database"
	"campfire/internal/database/repository"
	"campfire/internal/model"
)

func TestExistingDatabaseCompatibility(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "panel.db")

	legacy, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = legacy.Close() }()

	_, err = legacy.ExecContext(ctx, `
		CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY);
		INSERT INTO schema_migrations(version) VALUES (1);
		CREATE TABLE sparks (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL COLLATE NOCASE UNIQUE,
			kind TEXT NOT NULL CHECK (kind IN ('local', 'remote')),
			created_at TEXT NOT NULL
		);
		CREATE UNIQUE INDEX sparks_one_local ON sparks(kind) WHERE kind = 'local';
		INSERT INTO sparks VALUES ('legacy', 'Existing Spark', 'local', '2026-10-13T12:00:00.123456789Z');
	`)
	if err != nil {
		t.Fatal(err)
	}

	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := database.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = db.Close() }()

	store := repository.NewSparkRepository(db.ORM())

	if db.ORM().Migrator().HasColumn(&database.Spark{}, "connected") {
		t.Fatal("connected must not be stored in the database")
	}

	spark, err := store.Get(ctx, "legacy")
	if err != nil {
		t.Fatal(err)
	}

	if spark.Name != "Existing Spark" || spark.Kind != model.SparkLocal || spark.CreatedAt.Format(time.RFC3339Nano) != "2026-10-13T12:00:00.123456789Z" {
		t.Fatalf("legacy record changed: %+v", spark)
	}

	if spark.Description != "" || spark.Disabled || spark.Connected {
		t.Fatalf("incorrect defaults after migration: %+v", spark)
	}

	input := model.Spark{ID: "new", Name: "New Spark", Kind: model.SparkRemote, CreatedAt: time.Now().UTC()}

	input.PortAllocations = []model.PortAllocation{}
	if err := store.Create(ctx, input); err != nil {
		t.Fatal(err)
	}

	got, err := store.Get(ctx, input.ID)
	if err != nil || !reflect.DeepEqual(got, input) {
		t.Fatalf("new record round trip: %+v %v", got, err)
	}

	input.ID = "another"

	input.Name = "EXISTING SPARK"
	if err := store.Create(ctx, input); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("legacy name uniqueness lost: %v", err)
	}

	input.Name = "Second local"

	input.Kind = model.SparkLocal
	if err := store.Create(ctx, input); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("legacy local uniqueness lost: %v", err)
	}
}
