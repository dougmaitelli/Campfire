package repository_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

func TestCaseInsensitiveNamesAndGeneratedRoleIDs(t *testing.T) {
	db, auth, _ := authStore(t)

	ctx := t.Context()
	if err := auth.Setup(ctx, database.User{Username: "Admin", PasswordHash: "encoded", Version: 1}, database.UserSession{Hash: "setup", CredentialVersion: 1}); err != nil {
		t.Fatal(err)
	}

	user, err := auth.FindCredential(ctx, "ADMIN")
	if err != nil || user == nil || user.Username != "Admin" {
		t.Fatal("case-insensitive login failed", err)
	}

	duplicate := database.User{Username: "ADMIN", Version: 1, RoleID: database.UserRole}
	if err := auth.Access().CreateUser(ctx, &duplicate); !errors.Is(err, repository.ErrAccessConflict) {
		t.Fatal("case-insensitive username uniqueness lost", err)
	}

	role := database.Role{Name: "Custom", Scope: "platform"}
	if err := auth.Access().SaveRole(ctx, &role); err != nil || role.ID <= database.OperatorRole {
		t.Fatal("custom role collided with built-in IDs", role.ID, err)
	}

	duplicateRole := database.Role{Name: "CUSTOM", Scope: "platform"}
	if err := auth.Access().SaveRole(ctx, &duplicateRole); !errors.Is(err, repository.ErrAccessConflict) {
		t.Fatal("case-insensitive role uniqueness lost", err)
	}

	if err := db.ORM().Omit("User").Create(&database.UserSession{Hash: "orphan", UserID: 999}).Error; !errors.Is(err, gorm.ErrForeignKeyViolated) {
		t.Fatal("missing user did not violate foreign key", err)
	}
}

func TestPlacementSchedulesAndStorageAcrossConnections(t *testing.T) {
	db, _, source := authStore(t)
	ctx := t.Context()
	nodes := repository.NewSparkRepository(db.ORM())

	node := model.Spark{ID: "node", Name: "Node", Kind: model.SparkRemote, CreatedAt: time.Now().UTC(), SparkSettings: model.SparkSettings{
		CPULimit: 1, MemoryMB: 1024, DiskMB: 2048,
		PortAllocations: []model.PortAllocation{{BindIP: "127.0.0.1", Start: 25565, End: 25565, Protocol: "tcp"}},
	}}
	if err := nodes.Create(ctx, node); err != nil {
		t.Fatal(err)
	}

	node.ID, node.Name = "duplicate", "NODE"
	if err := nodes.Create(ctx, node); !errors.Is(err, repository.ErrConflict) {
		t.Fatal("case-insensitive Spark uniqueness lost", err)
	}

	ember := model.Ember{ID: "ember", Schema: 1, Name: "Game", Images: map[string]string{"Game": "alpine:3.22"}, Startup: "sleep {{SECONDS}}"}
	if err := repository.NewEmberRepository(db.ORM()).Create(ctx, ember); err != nil {
		t.Fatal(err)
	}

	second, err := database.Open(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close() }()

	results := make(chan error, 2)

	var wait sync.WaitGroup
	for i, orm := range []*gorm.DB{db.ORM(), second.ORM()} {
		wait.Add(1)
		go func() {
			defer wait.Done()

			id := []string{"first", "second"}[i]
			server := model.Server{ID: id, CreatedAt: time.Now().UTC(), ServerConfiguration: model.ServerConfiguration{
				Name: id, SparkID: "node", EmberID: "ember", Image: "alpine:3.22", CPULimit: 1, MemoryMB: 128, DiskMB: 1024,
				Variables: map[string]string{"SECONDS": "3600"}, Ports: []model.ServerPort{{BindIP: "127.0.0.1", HostPort: 25565, ContainerPort: 25565, Protocol: "tcp"}},
			}}

			results <- repository.NewServerRepository(orm).Save(ctx, server, true)
		}()
	}

	wait.Wait()
	close(results)

	var successes, conflicts int

	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, repository.ErrServerConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}

	if successes != 1 || conflicts != 1 {
		t.Fatal("concurrent placement overbooked a Spark", successes, conflicts)
	}

	servers, err := repository.NewServerRepository(second.ORM()).List(ctx)
	if err != nil || len(servers) != 1 || servers[0].Variables["SECONDS"] != "3600" || len(servers[0].Ports) != 1 || servers[0].EmberName != "Game" {
		t.Fatal("server JSON or associations did not round-trip", servers, err)
	}

	id := servers[0].ID
	activity := repository.NewActivityRepository(db.ORM())

	schedule := database.Schedule{ID: "nightly", ServerID: id, Name: "Restart", Action: "restart", IntervalMinutes: 60, Enabled: true, NextRunAt: time.Now().Add(-time.Minute).Unix(), Version: 1}
	if err := activity.SaveSchedule(ctx, &schedule, true); err != nil {
		t.Fatal(err)
	}

	if err := activity.EnqueueDue(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}

	runs, err := activity.Pending(ctx)
	if err != nil || len(runs) != 1 || !runs[0].Recurring {
		t.Fatal("recurring schedule was not persisted", runs, err)
	}

	backups := repository.NewBackupRepository(db.ORM())

	policy := model.BackupPolicy{Storage: "s3", LocalKeepLast: 3, RemoteKeepLast: 1}
	if err := backups.SavePolicy(ctx, id, policy); err != nil {
		t.Fatal(err)
	}

	if got, err := backups.Policy(ctx, id); err != nil || got != policy {
		t.Fatal("backup policy upsert failed", got, err)
	}

	for _, transfer := range []database.ServerTransfer{
		{ID: "old", ServerID: id, Active: false},
		{ID: "current", ServerID: id, Active: true},
		{ID: "conflict", ServerID: id, Active: true},
	} {
		err := db.ORM().Create(&transfer).Error
		if transfer.ID == "conflict" {
			if !errors.Is(err, gorm.ErrDuplicatedKey) {
				t.Fatal("multiple active transfers accepted", err)
			}
		} else if err != nil {
			t.Fatal("transfer partial index rejected valid history", err)
		}
	}
}
