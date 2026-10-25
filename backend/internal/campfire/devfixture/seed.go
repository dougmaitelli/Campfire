//go:build !production

// Package devfixture defines the shared sample world for development and browser tests.
package devfixture

import (
	"context"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/recurrence"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

// AdminName is shared by both views of the sample world.
const AdminName = "Developer"

// Server combines a saved server with its initial simulated lifecycle state.
type Server struct {
	model.Server
	State string
}

// Seed populates a fresh database after administrator setup. The caller chooses
// the clock and storage lifetime; fixture identities and content are shared.
func Seed(ctx context.Context, tx *gorm.DB, now time.Time) ([]Server, error) {
	tx = tx.WithContext(ctx)
	if err := tx.Model(&database.User{}).Where("id = ?", 1).Updates(map[string]any{"name": AdminName, "email": "admin@example.test", "created_at": now.Unix()}).Error; err != nil {
		return nil, err
	}

	var admin database.User
	if err := tx.Take(&admin, 1).Error; err != nil {
		return nil, err
	}

	team := database.Role{Name: "Community managers", Scope: "platform", Permissions: []database.RolePermission{{Permission: "servers.create"}, {Permission: "audit.view"}}}

	maintainers := database.Role{Name: "World maintainers", Scope: "server", Permissions: []database.RolePermission{{Permission: "server.view"}, {Permission: "server.operate"}, {Permission: "server.configure"}}}
	for _, role := range []*database.Role{&team, &maintainers} {
		if err := tx.Create(role).Error; err != nil {
			return nil, err
		}
	}

	for _, account := range []struct {
		username, name string
		role           int64
		disabled       bool
	}{
		{"alex", "Alex Rivera", database.CreatorRole, false}, {"sam", "Sam Chen", database.CreatorRole, false},
		{"jordan", "Jordan Patel", database.UserRole, false}, {"morgan", "Morgan Lee", team.ID, false},
		{"casey", "", database.UserRole, false}, {"taylor", "Taylor Brooks", database.UserRole, true},
		{"viewer", "Riley Quinn", database.UserRole, false},
	} {
		user := database.User{Username: account.username, Name: account.name, Email: account.username + "@example.test", PasswordHash: admin.PasswordHash, CredentialVersion: 1, RoleID: account.role, Disabled: account.disabled, CreatedAt: now.Unix()}
		if account.username == "sam" {
			user.OIDCNameManaged, user.OIDCEmailManaged = true, true
		}

		if err := tx.Create(&user).Error; err != nil {
			return nil, err
		}

		if account.username == "sam" {
			if err := tx.Create(&database.OIDCIdentity{Issuer: "https://identity.example.test", Subject: "mock-sam", UserID: user.ID}).Error; err != nil {
				return nil, err
			}
		}
	}

	for _, node := range []struct{ id, name, description string }{
		{"mock-europe", "Europe", "Primary game server node"}, {"mock-america", "America", "Secondary game server node"},
		{"mock-offline", "Offline", "Disconnected node for unavailable-state testing"},
	} {
		input := model.Spark{ID: node.id, Name: node.name, Description: node.description, CreatedAt: now, SparkSettings: model.SparkSettings{
			GameAddress: "127.0.0.1", CPULimit: 16, MemoryMB: 32768, DiskMB: 409600,
			PortAllocations: []model.PortAllocation{{BindIP: "0.0.0.0", Start: 25565, End: 25600, Protocol: "tcp"}, {BindIP: "0.0.0.0", Start: 25565, End: 25600, Protocol: "udp"}},
		}}
		if err := repository.NewSparkRepository(tx).Create(ctx, input); err != nil {
			return nil, err
		}
	}

	ember := model.Ember{Schema: 1, ID: "mock-minecraft", Name: "Minecraft", Description: "A Minecraft Java Edition server", ArtURL: "/game-art/mock-block.svg", Images: map[string]string{"Java 21": "ghcr.io/ptero-eggs/yolks:java_21"}, Startup: "java -Xmx{{SERVER_MEMORY}}M -jar server.jar", WorkDir: "/home/container", User: "1000:1000", Ready: "Done", Stop: "stop", Variables: MinecraftVariables(), RequiredPorts: []model.RequiredPort{{Container: 25565, Protocol: "tcp", Description: "Main"}}}
	if err := repository.NewEmberRepository(tx).Create(ctx, ember); err != nil {
		return nil, err
	}

	servers := make([]Server, 0, 4)
	for i, item := range []struct{ id, name, spark, state string }{
		{"survival", "Survival", "mock-europe", "running"}, {"creative", "Creative", "mock-europe", "stopped"},
		{"modded", "Modded", "mock-america", "running"}, {"unavailable", "Unavailable", "mock-offline", "stopped"},
	} {
		variables := make(map[string]string)
		for _, variable := range ember.Variables {
			variables[variable.Name] = variable.Default
		}

		configuration := model.ServerConfiguration{Name: item.name, Description: "Community Minecraft server", SparkID: item.spark, EmberID: ember.ID, Image: ember.Images["Java 21"], Variables: variables, CPULimit: 2, MemoryMB: 4096, DiskMB: 20480, Ports: []model.ServerPort{{BindIP: "0.0.0.0", HostPort: 25565 + i, ContainerPort: 25565, Protocol: "tcp"}}}

		server := model.Server{ID: item.id, ServerConfiguration: configuration, EmberHash: model.EmberHash(ember), CreatedAt: now, OwnerUserID: 1}
		if err := repository.NewServerRepository(tx).Save(ctx, server, true); err != nil {
			return nil, err
		}

		servers = append(servers, Server{Server: server, State: item.state})
	}

	for i, event := range []model.GameEvent{
		{ID: "fixture-connected", Type: "game.announcement", Title: "Game integration connected", Data: map[string]any{"game": "Minecraft"}},
		{ID: "fixture-joined", Type: "player.joined", Title: "Alex joined the server", Data: map[string]any{"player": "Alex"}},
		{ID: "fixture-advancement", Type: "minecraft.advancement", Title: "Alex earned Stone Age", Data: map[string]any{"player": "Alex", "advancement": "Stone Age"}},
	} {
		event.OccurredAt = now.Add(time.Duration(i-3) * time.Minute)
		if err := repository.NewGameEventRepository(tx).Append(ctx, "mock-europe", "survival", event, now, 30*24*time.Hour, 10000); err != nil {
			return nil, err
		}
	}

	if err := SeedGameMedia(ctx, tx); err != nil {
		return nil, err
	}

	next, err := recurrence.Next("0 3 * * *", "UTC", now, 1)
	if err != nil {
		return nil, err
	}

	if err := tx.Omit("Server").Create(&database.Schedule{ID: "nightly-restart", ServerID: "survival", Name: "Nightly restart", Action: "restart", CronExpression: "0 3 * * *", TimeZone: "UTC", OnlyWhenRunning: true, NextRunAt: next[0], Enabled: true, RunAsUserID: 1, CreatedAt: now.Add(-24 * time.Hour).Unix()}).Error; err != nil {
		return nil, err
	}

	for _, run := range []database.ScheduleRun{
		{ID: "scheduled-run", ScheduleID: "nightly-restart", ServerID: "survival", ServerName: "Survival", UserID: 1, Username: "admin", Action: "restart", Recurring: true, State: "succeeded", CreatedAt: now.Add(-9 * time.Hour).Unix(), StartedAt: now.Add(-9 * time.Hour).Unix(), FinishedAt: now.Add(-9*time.Hour + time.Minute).Unix()},
		{ID: "direct-operation", ServerID: "survival", ServerName: "Survival", UserID: 1, Username: "admin", Action: "stop", State: "succeeded", CreatedAt: now.Add(-time.Hour).Unix(), FinishedAt: now.Add(-time.Hour).Unix()},
	} {
		if err := repository.NewActivityRepository(tx).CreateRun(ctx, &run); err != nil {
			return nil, err
		}
	}

	actor := int64(1)
	if err := repository.NewActivityRepository(tx).BeginAudit(ctx, &database.AuditEvent{ID: "server-started", UserID: &actor, Username: "admin", Action: "server.start", Target: "server:survival", Outcome: "succeeded", Status: 202, CreatedAt: now.Add(-time.Hour).Unix()}); err != nil {
		return nil, err
	}

	inbox := repository.NewNotificationRepository(tx)

	for i, item := range []database.Notification{
		{TargetType: "server", TargetID: "survival", Kind: "backup", Severity: "info", Title: "Nightly backup completed", Body: "Survival was backed up successfully. The local archive is ready to download."},
		{TargetType: "spark", TargetID: "mock-offline", Kind: "health", Severity: "warning", Title: "Offline Spark", Body: "The node is not reporting. Check its Spark connection before scheduling new work."},
		{TargetType: "account", TargetID: "1", Kind: "schedule", Severity: "info", Title: "Nightly restart scheduled", Body: "Survival will restart daily at 03:00 UTC."},
	} {
		item.CreatedAt = now.Add(-time.Duration(3-i) * time.Hour).Unix()
		if err := inbox.Emit(ctx, &item, false); err != nil {
			return nil, err
		}

		if i == 0 {
			if err := tx.Model(&database.NotificationRecipient{}).Where("notification_id = ? AND user_id = ?", item.ID, 1).Update("read_at", now.Add(-2*time.Hour).Unix()).Error; err != nil {
				return nil, err
			}
		}
	}

	return servers, nil
}

// Backups identifies the same initial archives in both simulators.
func Backups(now time.Time) []model.FileEntry {
	return []model.FileEntry{
		{Name: "mock-nightly.tar.gz", Kind: "file", ModifiedAt: now.Add(-24 * time.Hour)},
		{Name: "mock-before-update.tar.gz", Kind: "file", ModifiedAt: now.Add(-48 * time.Hour)},
	}
}
