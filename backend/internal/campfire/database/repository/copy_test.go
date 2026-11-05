package repository_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
)

func TestCopyCreationReservationsAndCancellationAreAtomic(t *testing.T) {
	db, _, _ := authStore(t)
	ctx := t.Context()

	spark := model.Spark{ID: "node", Name: "Node", CreatedAt: time.Now().UTC(), SparkSettings: model.SparkSettings{MemoryMB: 256, PortAllocations: []model.PortAllocation{{BindIP: "0.0.0.0", Start: 25565, End: 25567, Protocol: model.NetworkTCP}}}}
	if err := repository.NewSparkRepository(db.ORM()).Create(ctx, spark); err != nil {
		t.Fatal(err)
	}

	embers := service.NewEmberService(repository.NewEmberRepository(db.ORM()))

	ember, err := embers.Import(ctx, model.Ember{Schema: 1, Name: "Game", Images: map[string]string{"default": "alpine:latest"}, Startup: "sleep 3600", WorkDir: "/home/container", User: "1000:1000", Variables: []model.Variable{{Name: "TOKEN", Default: "default-token", Editable: true, Hidden: true}}})
	if err != nil {
		t.Fatal(err)
	}

	servers := service.NewServerService(repository.NewServerRepository(db.ORM()), embers)
	configuration := model.ServerConfiguration{Name: "Original", SparkID: spark.ID, EmberID: ember.ID, Image: "alpine:latest", CPULimit: 1, MemoryMB: 128, Variables: map[string]string{"TOKEN": "private-value"}, Ports: []model.ServerPort{{BindIP: "0.0.0.0", HostPort: 25565, ContainerPort: 27015, Protocol: model.NetworkTCP}}}

	source, err := servers.Create(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}

	copies := repository.NewCopyRepository(db.ORM())
	create := func(store *repository.ServerRepository, emberStore *repository.EmberRepository, input model.ServerConfiguration) (model.Server, error) {
		return service.NewServerService(store, service.NewEmberService(emberStore)).Create(ctx, input)
	}

	target, err := copies.Create(ctx, source, ember, configuration, true, true, nil, create)
	if err != nil {
		t.Fatal(err)
	}

	if target.ID == source.ID || target.Name != "Original (copy)" || target.Ports[0].HostPort != 25566 || target.Ports[0].ContainerPort != 27015 {
		t.Fatal("copy identity or available port selection is incorrect", target)
	}

	for _, id := range []string{source.ID, target.ID} {
		if err := copies.Guard(ctx, id); !errors.Is(err, repository.ErrJobActive) {
			t.Fatal("copy reservation is missing", id, err)
		}
	}

	configuration.Name, configuration.MemoryMB, configuration.Ports = "Capacity probe", 64, nil
	if _, err := servers.Create(ctx, configuration); !errors.Is(err, repository.ErrServerConflict) {
		t.Fatal("copy did not reserve destination capacity", err)
	}

	jobs, err := copies.Jobs().List(ctx, source.ID, []model.JobType{model.JobCopy}, nil)
	if err != nil || len(jobs) != 1 {
		t.Fatal("copy history is missing", jobs, err)
	}

	encoded, err := json.Marshal(jobs[0])
	if err != nil || strings.Contains(string(encoded), "private-value") || strings.Contains(string(encoded), "default-token") {
		t.Fatal("copy progress exposed private snapshots", err)
	}

	if err := copies.Change(ctx, jobs[0].ID, true); err != nil {
		t.Fatal(err)
	}

	job, err := copies.Jobs().Get(ctx, jobs[0].ID)
	if err != nil || job.Status != model.JobCancelled {
		t.Fatal("queued cancellation did not finish immediately", job, err)
	}

	if err := copies.Guard(ctx, source.ID); err != nil {
		t.Fatal("cancelled copy retained the source lock", err)
	}

	var policies int64
	if err := db.ORM().Model(&database.BackupPolicy{}).Where("server_id = ?", target.ID).Count(&policies).Error; err != nil || policies != 0 {
		t.Fatal("cancelled copy retained its backup policy", policies, err)
	}

	configuration = source.ServerConfiguration
	configuration.Name, configuration.Ports = "Rolled back copy", nil
	failure := errors.New("creation interrupted")

	_, err = copies.Create(ctx, source, ember, configuration, false, true, nil, func(store *repository.ServerRepository, emberStore *repository.EmberRepository, input model.ServerConfiguration) (model.Server, error) {
		created, err := create(store, emberStore, input)
		if err != nil {
			return created, err
		}

		return created, failure
	})
	if !errors.Is(err, failure) {
		t.Fatal("expected transaction rollback", err)
	}

	remaining, err := servers.List(ctx)
	if err != nil || len(remaining) != 1 || remaining[0].ID != source.ID {
		t.Fatal("failed copy left partial reservations", remaining, err)
	}
}
