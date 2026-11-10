//go:build !production

package devmock

import (
	"context"
	"slices"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/devfixture"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
	accessperm "campfire/internal/shared/permissions"

	"gorm.io/gorm"
)

const configurationSeedMarkerID = 14
const configurationSparkID = "mock-configuration"
const restartExampleID = "mock-restart-required"
const syncExampleID = "mock-sync-pending"
const firstSyncExampleID = "mock-first-sync"

// seedConfigurationScenarios mixes configuration states into the existing fleet.
// Only mutually exclusive lifecycle/delivery states need additional servers.
func seedConfigurationScenarios(ctx context.Context, db *gorm.DB) error {
	return database.Transaction(db.WithContext(ctx), func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&seedMarker{}).Where("id = ?", configurationSeedMarkerID).Count(&count).Error; err != nil || count > 0 {
			return err
		}

		if err := repository.NewSparkRepository(tx).Create(ctx, model.Spark{
			ID: configurationSparkID, Name: "Configuration recovery", Description: "Connected mock Spark with paused configuration delivery. Run mock-config resume in Deployment sync · Mock's console to recover both servers.",
			CreatedAt: time.Now().UTC(), SparkSettings: model.SparkSettings{
				GameAddress: "games.example.test", CPULimit: 8, MemoryMB: 16384, DiskMB: 102400,
				PortAllocations: []model.PortAllocation{{BindIP: "0.0.0.0", Start: 25565, End: 25600, Protocol: model.NetworkTCP}},
			},
		}); err != nil {
			return err
		}

		if err := tx.Create(&sparkClientRecord{SparkID: configurationSparkID, Version: "v1.1.0", ConfigurationBlocked: true}).Error; err != nil {
			return err
		}

		if err := seedRestartExample(ctx, tx); err != nil {
			return err
		}

		if err := seedSyncExamples(ctx, tx); err != nil {
			return err
		}

		// Creative already covers stopped lifecycle, log alerts, named startup
		// choices, both backup modes, and retention. Add a metadata-only change.
		embers := repository.NewEmberRepository(tx)

		ember, err := embers.Get(ctx, devfixture.CurrentMinecraftEmberID)
		if err != nil {
			return err
		}

		if !slices.ContainsFunc(ember.SavedataPaths, func(path model.SavedataPath) bool { return path.Path == "whitelist.json" }) {
			ember.SavedataPaths = append(ember.SavedataPaths, model.SavedataPath{Path: "whitelist.json", Required: true})
		}

		if err := embers.Replace(ctx, ember); err != nil {
			return err
		}

		if err := seedConfigurationAccess(tx); err != nil {
			return err
		}

		return tx.Create(&seedMarker{ID: configurationSeedMarkerID}).Error
	})
}

func configurationEmber(id, name string) model.Ember {
	ember := mockUpdateCandidate(true)
	ember.ID, ember.Name, ember.UpdateURL = id, name, ""
	ember.Description = "Local configuration scenario; no game software is downloaded or executed."

	return ember
}

func seedRestartExample(ctx context.Context, tx *gorm.DB) error {
	ember := configurationEmber("mock-restart-ember", "Custom startup · Mock")
	if err := repository.NewEmberRepository(tx).Create(ctx, ember); err != nil {
		return err
	}

	input := model.ServerConfiguration{
		Name: "Restart required · Mock", Description: "Saved custom startup and memory differ from the running container. Restart to apply them. Game integration is disabled and disk capacity is unlimited.",
		SparkID: "mock-europe", EmberID: ember.ID, Image: ember.Images["Java 21"], StartupCommand: "Debug", StartupOverride: "java -Xmx{{SERVER_MEMORY}}M -Dexample=updated -jar server.jar",
		CPULimit: 1, MemoryMB: 2048, DiskMB: 0,
		Ports: []model.ServerPort{{BindIP: "0.0.0.0", HostPort: 25582, ContainerPort: 25565, Protocol: model.NetworkTCP}},
	}

	server, err := seedConfigurationServer(ctx, tx, restartExampleID, input)
	if err != nil {
		return err
	}

	desired, err := model.NewDeployment(server.ID, server.ServerConfiguration, ember)
	if err != nil {
		return err
	}

	active := desired
	active.Configuration.MemoryMB = 1024
	active.Ember.Startup = "java -Xmx{{SERVER_MEMORY}}M -jar server.jar"

	server.EmberHash = model.DeploymentHash(desired)
	if err := repository.NewServerRepository(tx).Save(ctx, server, false); err != nil {
		return err
	}

	record := newRecord(server.SparkID, desired)
	record.State, record.StartedAt = model.RuntimeStateRunning, time.Now().Add(-time.Hour).Unix()
	record.ActiveRevision = model.ContainerRevision(active)

	return tx.Create(&record).Error
}

func seedSyncExamples(ctx context.Context, tx *gorm.DB) error {
	before := configurationEmber("mock-sync-ember", "Deployment synchronization · Mock")
	after := configurationEmber(before.ID, before.Name)

	after.StartupCommands["Standard"] += " --mock-updated"
	if err := repository.NewEmberRepository(tx).Create(ctx, after); err != nil {
		return err
	}

	for _, example := range []struct {
		id, name, description string
		state                 model.RuntimeState
		port                  int
	}{
		{syncExampleID, "Deployment sync · Mock", "Running on a connected Spark that rejects configuration delivery. Status, console and backups work. Run mock-config resume here, then sync or wait for retry.", model.RuntimeStateRunning, 25565},
		{firstSyncExampleID, "First sync · Mock", "Not installed and awaiting its first configuration acknowledgement. Transfer can be queued. Recover delivery from Deployment sync · Mock's console, then install normally.", model.RuntimeStateNotInstalled, 25566},
	} {
		server, err := seedConfigurationServer(ctx, tx, example.id, model.ServerConfiguration{
			Name: example.name, Description: example.description, SparkID: configurationSparkID, EmberID: after.ID, Image: after.Images["Java 21"], StartupCommand: "Standard",
			CPULimit: 1, MemoryMB: 1024, DiskMB: 4096,
			Ports: []model.ServerPort{{BindIP: "0.0.0.0", HostPort: example.port, ContainerPort: 25565, Protocol: model.NetworkTCP}},
		})
		if err != nil {
			return err
		}

		deployed, err := model.NewDeployment(server.ID, server.ServerConfiguration, before)
		if err != nil {
			return err
		}

		if example.state == model.RuntimeStateRunning {
			server.EmberHash = model.DeploymentHash(deployed)
			if err := repository.NewServerRepository(tx).Save(ctx, server, false); err != nil {
				return err
			}
		}

		record := newRecord(server.SparkID, deployed)

		record.State = example.state
		if record.State == model.RuntimeStateRunning {
			record.ActiveRevision = model.ContainerRevision(deployed)
			record.StartedAt = time.Now().Add(-time.Hour).Unix()
		}

		if err := tx.Create(&record).Error; err != nil {
			return err
		}
	}

	return nil
}

func seedConfigurationServer(ctx context.Context, tx *gorm.DB, id string, input model.ServerConfiguration) (model.Server, error) {
	servers := service.NewServerService(repository.NewServerRepository(tx), service.NewEmberService(repository.NewEmberRepository(tx)))

	server, err := servers.CreateOwned(ctx, input, 1)
	if err != nil {
		return model.Server{}, err
	}

	// Fixtures keep stable resource links. Renaming inside this seed transaction
	// precedes runtime rows, access grants and background collection.
	if err := tx.Model(&database.Server{}).Where("id = ?", server.ID).Update("id", id).Error; err != nil {
		return model.Server{}, err
	}

	server.ID = id

	return server, nil
}

func seedConfigurationAccess(tx *gorm.DB) error {
	reader := database.Role{Name: "Status readers", Scope: model.RoleScopeServer, Permissions: []database.RolePermission{{Permission: accessperm.ServerView}}}
	if err := tx.Create(&reader).Error; err != nil {
		return err
	}

	for _, access := range []struct {
		username, server string
		role             int64
	}{
		{"casey", restartExampleID, reader.ID},
		{"casey", syncExampleID, reader.ID},
		{"sam", syncExampleID, database.OperatorRole},
	} {
		var user database.User
		if err := tx.Take(&user, "username = ?", access.username).Error; err != nil {
			return err
		}

		if err := tx.Omit("Server", "User", "Role", "AccessRole").Create(&database.ServerAccessGrant{ServerID: access.server, UserID: &user.ID, AccessRoleID: access.role}).Error; err != nil {
			return err
		}
	}

	return nil
}
