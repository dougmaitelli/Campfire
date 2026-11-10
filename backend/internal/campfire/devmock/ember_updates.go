//go:build !production

package devmock

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/devfixture"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

const mockPendingUpdateURL = "https://mock.campfire.invalid/minecraft.json"
const mockCurrentUpdateURL = "https://mock.campfire.invalid/current.json"
const mockImportedUpdateID = devfixture.MinecraftEmberID
const mockUpdateServerName = "Ember update ready · Mock"

func mockUpdateCandidate(current bool) model.Ember {
	ember := model.Ember{Schema: 1, Name: "Minecraft · Mock", Description: "Updated development fixture. No image is pulled and no game process runs.", UpdateURL: mockPendingUpdateURL, ArtURL: "/game-art/mock-block.svg", Images: map[string]string{"Java 21": "ghcr.io/ptero-eggs/yolks:java_21"}, Startup: "java -Xmx{{SERVER_MEMORY}}M -jar server.jar", WorkDir: "/home/container", User: "1000:1000", Ready: "Done", Stop: "stop", Variables: []model.Variable{}, RequiredPorts: []model.RequiredPort{{Container: 25565, Protocol: model.NetworkTCP, Description: "Main"}}, Installer: &model.Installer{Image: "alpine:3.22", Entrypoint: "sh", Script: "echo 'Mock updated installer; no game downloads.'"}}
	if current {
		ember.Name = "Minecraft · Current"
		ember.UpdateURL = mockCurrentUpdateURL
		ember.StartupCommands = map[string]string{
			"Standard": ember.Startup,
			"Debug":    "java -Xmx{{SERVER_MEMORY}}M -Ddebug=true -jar server.jar",
		}
		ember.Startup = ""
		ember.SavedataPaths = []model.SavedataPath{{Path: "world", Required: true}, {Path: "server.properties", Required: true}, {Path: "config"}, {Path: "whitelist.json", Required: true}}
	}

	ember.Variables = mockMinecraftVariables()
	ember.LogAlerts = mockLogAlerts()

	return ember
}

// FetchEmberUpdate serves fixed development definitions without making network requests.
func (r *Runtime) FetchEmberUpdate(ctx context.Context, address string, options service.ImportOptions) (service.EmberPreview, error) {
	if err := ctx.Err(); err != nil {
		return service.EmberPreview{}, err
	}

	if address != mockPendingUpdateURL && address != mockCurrentUpdateURL {
		return service.EmberPreview{}, fmt.Errorf("%w: no mock update is configured for this URL", service.ErrInvalidEmber)
	}

	data, err := json.Marshal(mockUpdateCandidate(address == mockCurrentUpdateURL))
	if err != nil {
		return service.EmberPreview{}, err
	}

	return service.PreviewEmber(data, options), nil
}

func seedEmberUpdates(ctx context.Context, db *gorm.DB) error {
	return database.Transaction(db.WithContext(ctx), func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&seedMarker{}).Where("id = ?", 4).Count(&count).Error; err != nil || count > 0 {
			return err
		}

		var original database.Ember
		if err := tx.Where("id = ?", devfixture.MinecraftEmberID).Take(&original).Error; err != nil {
			return err
		}

		original.Definition.UpdateURL = mockPendingUpdateURL
		if err := tx.Model(&original).Select("Definition").Updates(original).Error; err != nil {
			return err
		}

		current := mockUpdateCandidate(true)
		// The metadata scenario adds this path after backup fixtures are created.
		current.SavedataPaths = slices.DeleteFunc(current.SavedataPaths, func(path model.SavedataPath) bool { return path.Path == "whitelist.json" })

		current.ID = devfixture.CurrentMinecraftEmberID
		if err := repository.NewEmberRepository(tx).Create(ctx, current); err != nil {
			return err
		}

		checks := repository.NewEmberUpdateRepository(tx)
		for _, item := range []model.EmberUpdate{
			{EmberID: original.ID, Available: true, CheckedAt: time.Now().Add(-2 * time.Hour).Unix()},
			{EmberID: current.ID, Available: false, CheckedAt: time.Now().Add(-2 * time.Hour).Unix()},
		} {
			if err := checks.Save(ctx, item); err != nil {
				return err
			}
		}

		return tx.Create(&seedMarker{ID: 4}).Error
	})
}

// seedServerEmberUpdate adds a running server with saved values awaiting review.
// Its own marker also upgrades existing mock databases without resetting edits.
func seedServerEmberUpdate(ctx context.Context, db *gorm.DB) error {
	return database.Transaction(db.WithContext(ctx), func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&seedMarker{}).Where("id = ?", 5).Count(&count).Error; err != nil || count > 0 {
			return err
		}

		emberStore := repository.NewEmberRepository(tx)

		original, err := emberStore.Get(ctx, devfixture.MinecraftEmberID)
		if err != nil {
			return err
		}

		original.Variables = append(original.Variables,
			model.Variable{Name: "INTERNAL_TOKEN", Default: "mock-old", Hidden: true},
			model.Variable{Name: "RETIRED_SETTING", Default: "legacy", Hidden: true},
		)

		updated := mockUpdateCandidate(false)

		updated.ID = original.ID
		updated.Images = map[string]string{"Java 25": "ghcr.io/ptero-eggs/yolks:java_25"}
		updated.StartupCommands = map[string]string{"Standard": updated.Startup}
		updated.Startup = ""
		updated.Variables = append(updated.Variables,
			model.Variable{Name: "EXTRA_ARGUMENTS", Editable: true},
			model.Variable{Name: "ENABLE_CROSSPLAY", Default: "1", Editable: true, Validation: &model.VariableValidation{Type: model.VariableBoolean}},
			model.Variable{Name: "INTERNAL_TOKEN", Default: "mock-new", Hidden: true},
		)

		updated.SavedataPaths = []model.SavedataPath{{Path: "world", Required: true}, {Path: "server.properties", Required: true}}
		for i := range updated.Variables {
			if updated.Variables[i].Name == "INSTALL_CHANNEL" {
				updated.Variables[i].Default = "preview"
			}
		}

		updated.Description += " Local update awaiting application."
		if err := emberStore.Replace(ctx, updated); err != nil {
			return err
		}

		if err := repository.NewEmberUpdateRepository(tx).Save(ctx, model.EmberUpdate{EmberID: updated.ID, CheckedAt: time.Now().Unix(), Available: true}); err != nil {
			return err
		}

		configuration := model.ServerConfiguration{
			Name: mockUpdateServerName, Description: "Running with saved values awaiting Ember review: image, startup selection, locked defaults, added settings and private changes. Both backup types remain available. Apply without stopping, then restart separately.",
			SparkID: "mock-europe", EmberID: original.ID, Image: original.Images["Java 21"],
			Variables: map[string]string{"MOTD": "Custom greeting retained during review"}, GameIntegration: true, CPULimit: 2, MemoryMB: 4096, DiskMB: 20480,
			Ports: []model.ServerPort{{BindIP: "0.0.0.0", HostPort: 25569, ContainerPort: 25565, Protocol: model.NetworkTCP}, {BindIP: "0.0.0.0", HostPort: 25570, ContainerPort: 27015, Protocol: model.NetworkUDP}},
		}
		servers := service.NewServerService(repository.NewServerRepository(tx), service.NewEmberService(emberStore))

		server, err := servers.CreateOwned(ctx, updated.ProposedServerConfiguration(configuration), 1)
		if err != nil {
			return err
		}

		server.ServerConfiguration = original.ProposedServerConfiguration(configuration)

		deployment, err := model.NewDeployment(server.ID, server.ServerConfiguration, original)
		if err != nil {
			return err
		}

		server.EmberHash = model.DeploymentHash(deployment)
		if err := repository.NewServerRepository(tx).Save(ctx, server, false); err != nil {
			return err
		}

		record := newRecord(configuration.SparkID, deployment)
		record.State = model.RuntimeStateRunning
		record.ActiveRevision = model.ContainerRevision(deployment)
		record.StartedAt = time.Now().Add(-2 * time.Hour).Unix()

		record.Logs += "[Mock Spark] The current local Ember definition is ready to apply.\n"
		if err := tx.Create(&record).Error; err != nil {
			return err
		}

		return tx.Create(&seedMarker{ID: 5}).Error
	})
}
