package repository_test

import (
	"errors"
	"testing"
	"time"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
)

func TestTransferCreationRejectsChangedInputs(t *testing.T) {
	for _, change := range []string{"server-value-review", "acknowledged-deployment", "configuration", "definition-only"} {
		t.Run(change, func(t *testing.T) {
			db, _, _ := authStore(t)
			nodes := repository.NewSparkRepository(db.ORM())

			for _, id := range []string{"source", "target"} {
				if err := nodes.Create(t.Context(), model.Spark{ID: id, Name: id, CreatedAt: time.Now().UTC()}); err != nil {
					t.Fatal(err)
				}
			}

			embers := service.NewEmberService(repository.NewEmberRepository(db.ORM()))

			ember, err := embers.Import(t.Context(), model.Ember{Schema: 1, Name: "Game", Images: map[string]string{"default": "alpine:latest"}, Startup: "sleep 3600", WorkDir: "/home/container", User: "1000:1000"})
			if err != nil {
				t.Fatal(err)
			}

			servers := service.NewServerService(repository.NewServerRepository(db.ORM()), embers)

			source, err := servers.Create(t.Context(), model.ServerConfiguration{Name: "Game", SparkID: "source", EmberID: ember.ID, Image: "alpine:latest", CPULimit: 1, MemoryMB: 128})
			if err != nil {
				t.Fatal(err)
			}

			target := source.ServerConfiguration
			target.SparkID = "target"
			work := model.TransferWork{ID: "job", ServerID: source.ID, SourceSparkID: "source", TargetSparkID: "target", SourceConfiguration: source.ServerConfiguration, TargetConfiguration: target, CreatedAt: time.Now().Unix()}

			if change != "configuration" {
				ember.Startup = "sleep 9876"
				if change == "server-value-review" {
					ember.Variables = []model.Variable{{Name: "NEW_VALUE", Default: "compatible", Editable: true}}
				}

				if _, err := embers.Replace(t.Context(), ember.ID, ember); err != nil {
					t.Fatal(err)
				}

				if change == "acknowledged-deployment" {
					deployment, err := model.NewDeployment(source.ID, source.ServerConfiguration, ember)
					if err != nil {
						t.Fatal(err)
					}

					applied := source
					applied.EmberHash = model.DeploymentHash(deployment)

					if err := repository.NewServerRepository(db.ORM()).Save(t.Context(), applied, false); err != nil {
						t.Fatal(err)
					}
				}
			} else {
				configuration := source.ServerConfiguration
				configuration.MemoryMB = 256

				if _, err := servers.Update(t.Context(), source.ID, configuration); err != nil {
					t.Fatal(err)
				}
			}

			transfers := repository.NewTransferRepository(db.ORM())

			err = transfers.Create(t.Context(), work, source.EmberHash)
			if change == "definition-only" {
				if err != nil {
					t.Fatal("compatible definition-only change blocked a pending-sync transfer", err)
				}

				return
			}

			if !errors.Is(err, repository.ErrJobState) {
				t.Fatal("transfer reserved stale inputs", err)
			}

			if err := transfers.Guard(t.Context(), source.ID); err != nil {
				t.Fatal("rejected transfer retained its reservations", err)
			}
		})
	}
}
