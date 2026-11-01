package repository_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

func TestServerConflictsReportNameResourcesAndPortsTogether(t *testing.T) {
	db, _, _ := authStore(t)
	ctx := t.Context()

	spark := model.Spark{ID: "node", Name: "Node", CreatedAt: time.Now().UTC(), SparkSettings: model.SparkSettings{CPULimit: 1, MemoryMB: 1024, DiskMB: 4096, PortAllocations: []model.PortAllocation{{BindIP: "127.0.0.1", Start: 25565, End: 25566, Protocol: model.NetworkTCP}}}}
	if err := repository.NewSparkRepository(db.ORM()).Create(ctx, spark); err != nil {
		t.Fatal(err)
	}

	ember := model.Ember{ID: "ember", Schema: 1, Name: "Game", Images: map[string]string{"default": "alpine:latest"}, Startup: "./server"}
	if err := repository.NewEmberRepository(db.ORM()).Create(ctx, ember); err != nil {
		t.Fatal(err)
	}

	store := repository.NewServerRepository(db.ORM())

	first := model.Server{ID: "first", CreatedAt: time.Now().UTC(), ServerConfiguration: model.ServerConfiguration{Name: "First server", SparkID: spark.ID, EmberID: ember.ID, Image: "alpine:latest", Variables: map[string]string{}, CPULimit: 0.75, MemoryMB: 768, DiskMB: 3072, Ports: []model.ServerPort{{BindIP: "127.0.0.1", HostPort: 25565, ContainerPort: 25565, Protocol: model.NetworkTCP}}}}
	if err := store.Save(ctx, first, true); err != nil {
		t.Fatal(err)
	}

	second := first
	second.ID, second.Name = "second", "FIRST SERVER"
	second.CPULimit, second.MemoryMB, second.DiskMB = 0.5, 512, 2048

	err := store.Save(ctx, second, true)
	if !errors.Is(err, repository.ErrServerConflict) {
		t.Fatal("missing conflict classification", err)
	}

	for _, message := range []string{"name already exists", "CPU capacity", "memory capacity", "disk capacity", "25565/tcp on 127.0.0.1"} {
		if !strings.Contains(err.Error(), message) {
			t.Error("missing specific conflict", message, err)
		}
	}

	rows, err := store.List(ctx)
	if err != nil || len(rows) != 1 {
		t.Fatal("failed creation modified reservations", rows, err)
	}

	second.Name = "Second server"
	second.CPULimit, second.MemoryMB, second.DiskMB = 0.1, 64, 512

	second.Ports = []model.ServerPort{{BindIP: "127.0.0.1", HostPort: 25566, ContainerPort: 25565, Protocol: model.NetworkTCP}}
	if err := store.Save(ctx, second, true); err != nil {
		t.Fatal(err)
	}

	second.Name = "FIRST SERVER"
	if err := store.Save(ctx, second, false); err == nil || !strings.Contains(err.Error(), "name already exists") {
		t.Fatal("metadata-only rename lost name conflict details", err)
	}
}
