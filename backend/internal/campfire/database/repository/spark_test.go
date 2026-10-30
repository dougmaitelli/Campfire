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

	input := model.Spark{ID: "node", Name: "Existing Spark", CreatedAt: time.Now().UTC(), SparkSettings: model.SparkSettings{PortAllocations: []model.PortAllocation{}}}
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

func TestSparkRenamePreservesIdentityAndAssignments(t *testing.T) {
	db, _, _ := authStore(t)
	ctx := t.Context()
	store := repository.NewSparkRepository(db.ORM())

	node := model.Spark{ID: "node", Name: "Original", CreatedAt: time.Now().UTC(), SparkSettings: model.SparkSettings{PortAllocations: []model.PortAllocation{}}}
	if err := store.Create(ctx, node); err != nil {
		t.Fatal(err)
	}

	ember := model.Ember{ID: "ember", Schema: 1, Name: "Game", Images: map[string]string{"Game": "alpine:3.22"}, Startup: "sleep 1"}
	if err := repository.NewEmberRepository(db.ORM()).Create(ctx, ember); err != nil {
		t.Fatal(err)
	}

	server := database.Server{ID: "game", Name: "Game", SparkID: node.ID, EmberID: ember.ID, Image: "alpine:3.22", Variables: map[string]string{}, Ports: []model.ServerPort{}, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if err := db.ORM().Omit("Spark", "Ember", "Owner").Create(&server).Error; err != nil {
		t.Fatal(err)
	}

	identity := database.SparkIdentity{SparkID: node.ID, CertificateHash: "certificate"}
	if err := db.ORM().Omit("Spark").Create(&identity).Error; err != nil {
		t.Fatal(err)
	}

	name := "Renamed"

	renamed, err := store.Update(ctx, node.ID, model.SparkChanges{Name: &name})
	if err != nil {
		t.Fatal(err)
	}

	node.Name = name
	if !reflect.DeepEqual(renamed, node) {
		t.Fatal("rename changed other Spark fields", renamed)
	}

	if !repository.NewEnrollmentRepository(db.ORM()).Authorized(ctx, node.ID, identity.CertificateHash) {
		t.Fatal("rename revoked the enrolled identity")
	}

	assigned, err := repository.NewServerRepository(db.ORM()).Get(ctx, server.ID)
	if err != nil || assigned.SparkID != node.ID || assigned.SparkName != name {
		t.Fatal("rename changed server placement or left a stale name", assigned, err)
	}

	other := node

	other.ID, other.Name = "other", "Other"
	if err := store.Create(ctx, other); err != nil {
		t.Fatal(err)
	}

	name = "OTHER"

	description := "Must not be saved"
	if _, err := store.Update(ctx, node.ID, model.SparkChanges{Name: &name, Description: &description}); !errors.Is(err, repository.ErrConflict) {
		t.Fatal("case-insensitive rename conflict lost", err)
	}

	unchanged, err := store.Get(ctx, node.ID)
	if err != nil || !reflect.DeepEqual(unchanged, node) {
		t.Fatal("conflicting rename was not atomic", unchanged, err)
	}
}
