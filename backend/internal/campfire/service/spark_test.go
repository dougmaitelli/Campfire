package service_test

import (
	"context"
	"path/filepath"
	"testing"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
)

type connectivity map[string]bool

func (c connectivity) Connected(id string) bool { return c[id] }

func TestCalculatedConnectivity(t *testing.T) {
	ctx := context.Background()

	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = db.Close() }()

	store := repository.NewSparkRepository(db.ORM())
	connections := make(connectivity)
	sparks := service.NewSparkService(store, connections)

	spark, err := sparks.Create(ctx, service.CreateSparkInput{Name: "node", Kind: model.SparkRemote})
	if err != nil {
		t.Fatal(err)
	}

	if spark.Connected {
		t.Fatal("unconnected node reported connected")
	}

	connections[spark.ID] = true

	spark, err = sparks.Get(ctx, spark.ID)
	if err != nil || !spark.Connected {
		t.Fatalf("live connection missing: %+v %v", spark, err)
	}

	stored, err := store.Get(ctx, spark.ID)
	if err != nil || stored.Connected {
		t.Fatalf("connection state persisted: %+v %v", stored, err)
	}

	list, err := sparks.List(ctx)
	if err != nil || len(list) != 1 || !list[0].Connected {
		t.Fatalf("list must calculate connectivity: %+v %v", list, err)
	}

	disabled := true

	spark, err = sparks.Update(ctx, spark.ID, service.UpdateSparkInput{Disabled: &disabled})
	if err != nil || !spark.Disabled || spark.Connected {
		t.Fatalf("disabled node reported connected: %+v %v", spark, err)
	}

	disabled = false

	spark, err = sparks.Update(ctx, spark.ID, service.UpdateSparkInput{Disabled: &disabled})
	if err != nil || spark.Disabled || !spark.Connected {
		t.Fatalf("re-enabled state incorrect: %+v %v", spark, err)
	}

	connections[spark.ID] = false

	spark, err = sparks.Get(ctx, spark.ID)
	if err != nil || spark.Connected {
		t.Fatalf("disconnection not reflected: %+v %v", spark, err)
	}
}
