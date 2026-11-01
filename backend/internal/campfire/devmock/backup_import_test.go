//go:build !production

package devmock

import (
	"context"
	"testing"
	"time"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
)

func TestPortableBackupPublishAndRestoreThroughMockServices(t *testing.T) {
	runtime, db, _, servers := fixture(t)

	var source model.Server

	for _, server := range servers {
		if server.ID == "creative" {
			source = server
		}
	}

	embers := service.NewEmberService(repository.NewEmberRepository(db.ORM()))
	serverService := service.NewServerService(repository.NewServerRepository(db.ORM()), embers)
	sparks := service.NewSparkService(repository.NewSparkRepository(db.ORM()), runtime)
	deployment := &service.DeploymentService{Servers: serverService, Sparks: sparks, Embers: embers, Remote: runtime}
	deployment.Backups = service.NewBackupService(repository.NewBackupRepository(db.ORM()), deployment, nil)

	archive, err := runtime.Files(t.Context(), source.SparkID, model.FileRequest{ServerID: source.ID, Operation: model.FileOperationBackupRead, Path: model.BackupModeFull.ArchiveName("mock-before-update")})
	if err != nil {
		t.Fatal(err)
	}

	configuration := source.ServerConfiguration
	configuration.Name = "Recovered server"
	configuration.Ports = nil
	configuration.CPULimit, configuration.MemoryMB, configuration.DiskMB = 0.1, 64, 0

	created, err := serverService.CreateOwned(t.Context(), configuration, source.OwnerUserID)
	if err != nil {
		t.Fatal(err)
	}

	imports := service.NewBackupImports()

	token, err := imports.Begin(source.OwnerUserID, created.ID, int64(len(archive.Data)), model.DefaultUploadLimit)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { imports.Cancel(source.OwnerUserID, token) })

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	var name string

	err = imports.With(source.OwnerUserID, token, func(upload *service.BackupUpload) error {
		if err := upload.Chunk(0, archive.Data); err != nil {
			return err
		}

		if err := upload.Inspect(ctx); err != nil {
			return err
		}

		name = upload.Name

		return upload.Publish(ctx, deployment, created.ID, func() error { return nil })
	})
	if err != nil {
		t.Fatal("publish failed", err)
	}

	listed, err := deployment.Backups.List(ctx, created.ID)
	if err != nil || len(listed) != 1 || !listed[0].Portable {
		t.Fatal("published backup missing from catalog", listed, err)
	}

	status, err := deployment.Restore(ctx, created.ID, name)
	if err != nil || status.State != model.RuntimeStateStopped {
		t.Fatal("restore failed", status, err)
	}
}
