//go:build !production

// Package devmock simulates Spark transports for local development without Docker.
package devmock

import (
	"context"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

// Username and Password are initial credentials for the isolated development panel.
const (
	Username = "admin"
	Password = "campfire-dev-password"
)

type seedMarker struct {
	ID int `gorm:"primaryKey;autoIncrement:false"`
}

// New seeds a fresh mock database once, then restores its saved simulated state.
func New(ctx context.Context, db *gorm.DB, auth *service.AuthService) (*Runtime, error) {
	if err := db.WithContext(ctx).AutoMigrate(&seedMarker{}, &runtimeRecord{}); err != nil {
		return nil, err
	}

	token, err := auth.PrepareSetup(ctx)
	if err != nil {
		return nil, err
	}

	if token != "" {
		if _, _, err := auth.Setup(ctx, token, Username, Password); err != nil {
			return nil, err
		}
	}

	err = database.Transaction(db.WithContext(ctx), func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&seedMarker{}).Count(&count).Error; err != nil || count > 0 {
			return err
		}

		if err := seed(ctx, tx); err != nil {
			return err
		}

		return tx.Create(&seedMarker{ID: 1}).Error
	})
	if err != nil {
		return nil, err
	}

	return &Runtime{db: db}, nil
}

func seed(ctx context.Context, tx *gorm.DB) error {
	if err := tx.Model(&database.User{}).Where("id = ?", 1).Update("name", "Developer").Error; err != nil {
		return err
	}

	sparks := repository.NewSparkRepository(tx)

	for _, node := range []struct{ id, name, description string }{
		{"mock-europe", "Europe · Mock", "Simulated online node for local development."},
		{"mock-america", "America · Mock", "Simulated online node with a second running server."},
		{"mock-offline", "Offline · Mock", "Always disconnected, for unavailable-state testing."},
	} {
		input := model.Spark{ID: node.id, Name: node.name, Description: node.description, Kind: model.SparkRemote, CreatedAt: time.Now().UTC(), SparkSettings: model.SparkSettings{
			GameAddress: "127.0.0.1", CPULimit: 16, MemoryMB: 32768, DiskMB: 409600,
			PortAllocations: []model.PortAllocation{{BindIP: "0.0.0.0", Start: 25565, End: 25600, Protocol: "tcp"}, {BindIP: "0.0.0.0", Start: 25565, End: 25600, Protocol: "udp"}},
		}}
		if err := sparks.Create(ctx, input); err != nil {
			return err
		}
	}

	ember := model.Ember{Schema: 1, ID: "mock-minecraft", Name: "Minecraft · Mock", Description: "Development fixture. No image is pulled and no game process runs.", Images: map[string]string{"Java 21": "ghcr.io/ptero-eggs/yolks:java_21"}, Startup: "java -Xmx{{SERVER_MEMORY}}M -jar server.jar", WorkDir: "/home/container", User: "1000:1000", Ready: "Done", Stop: "stop", Variables: []model.Variable{}, RequiredPorts: []model.RequiredPort{{Container: 25565, Protocol: "tcp"}}}
	embers := service.NewEmberService(repository.NewEmberRepository(tx))

	if err := repository.NewEmberRepository(tx).Create(ctx, ember); err != nil {
		return err
	}

	servers := service.NewServerService(repository.NewServerRepository(tx), embers)

	for index, fixture := range []struct{ name, node, state string }{
		{"Survival · Mock", "mock-europe", "running"},
		{"Creative · Mock", "mock-europe", "stopped"},
		{"Modded · Mock", "mock-america", "running"},
		{"Unavailable · Mock", "mock-offline", "stopped"},
	} {
		configuration := model.ServerConfiguration{Name: fixture.name, Description: "Simulated server. Actions and file edits affect only this mock database.", SparkID: fixture.node, EmberID: ember.ID, Image: ember.Images["Java 21"], Variables: map[string]string{}, CPULimit: 2, MemoryMB: 4096, DiskMB: 20480, Ports: []model.ServerPort{{BindIP: "0.0.0.0", HostPort: 25565 + index, ContainerPort: 25565, Protocol: "tcp"}}}

		server, err := servers.CreateOwned(ctx, configuration, 1)
		if err != nil {
			return err
		}

		record := newRecord(fixture.node, model.Deployment{ServerID: server.ID, Configuration: configuration, Ember: ember})
		record.State = fixture.state

		if fixture.state == "running" {
			record.Logs += "[Server] Done! Mock server ready for commands.\n"
		}

		if err := tx.Create(&record).Error; err != nil {
			return err
		}

		if index == 0 {
			schedule := database.Schedule{ID: "mock-restart", ServerID: server.ID, Name: "Daily restart · Mock", Action: "restart", IntervalMinutes: 1440, NextRunAt: time.Now().Add(24 * time.Hour).Unix(), Enabled: true, UserID: 1, Username: Username, Version: 1}
			if err := tx.Omit("Server").Create(&schedule).Error; err != nil {
				return err
			}
		}
	}

	inbox := repository.NewNotificationRepository(tx)

	for _, item := range []database.Notification{
		{TargetType: "account", TargetID: "1", Kind: "test", Severity: "info", Title: "Welcome to mock mode", Body: "Sparks, metrics and game processes are simulated. Your production infrastructure is not connected."},
		{TargetType: "spark", TargetID: "mock-offline", Kind: "health", Severity: "warning", Title: "Offline Spark · Mock", Body: "This fixture stays offline so you can develop unavailable and recovery views."},
	} {
		item.CreatedAt = time.Now().Unix()
		if err := inbox.Emit(ctx, &item, false); err != nil {
			return err
		}
	}

	return nil
}
