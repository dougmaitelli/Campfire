package repository_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

func TestSparkPersistenceAcrossReopen(t *testing.T) {
	ctx := t.Context()
	db, _, source := authStore(t)
	store := repository.NewSparkRepository(db.ORM())

	input := model.Spark{ID: "node", Name: "Existing Spark", Kind: model.SparkRemote, CreatedAt: time.Now().UTC(), SparkSettings: model.SparkSettings{PortAllocations: []model.PortAllocation{}}}
	if err := store.Create(ctx, input); err != nil {
		t.Fatal(err)
	}

	if db.ORM().Migrator().HasColumn(&database.Spark{}, "connected") {
		t.Fatal("connected must not be stored in the database")
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := database.Open(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()

	store = repository.NewSparkRepository(reopened.ORM())

	got, err := store.Get(ctx, input.ID)
	if err != nil || !reflect.DeepEqual(got, input) {
		t.Fatalf("Spark record round trip: %+v %v", got, err)
	}

	input.ID = "another"
	input.Name = "EXISTING SPARK"

	if err := store.Create(ctx, input); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("case-insensitive name uniqueness lost: %v", err)
	}

	input.Name = "Second agent"
	if err := store.Create(ctx, input); err != nil {
		t.Fatalf("multiple agent registrations rejected: %v", err)
	}
}
