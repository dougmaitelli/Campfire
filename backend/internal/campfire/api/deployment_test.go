package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"campfire/internal/campfire/api"
	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
	"campfire/internal/spark/agent"
)

func TestConfigurationDeliveryPlacementAndSnapshot(t *testing.T) {
	for _, kind := range []string{"remote"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()

			db, err := database.Open(context.Background(), filepath.Join(dir, "panel.db"))
			if err != nil {
				t.Fatal(err)
			}

			t.Cleanup(func() { _ = db.Close() })

			sparks := service.NewSparkService(repository.NewSparkRepository(db.ORM()), nil)
			embers := service.NewEmberService(repository.NewEmberRepository(db.ORM()))
			servers := service.NewServerService(repository.NewServerRepository(db.ORM()), embers)
			deployment := &service.DeploymentService{Servers: servers, Sparks: sparks, Embers: embers, Remote: &agent.ConfigurationStore{Directory: filepath.Join(dir, "remote")}}
			handler := authenticatedTestAPI(t, db, api.New(sparks, embers, servers).WithDeployment(deployment))
			node, ember, input := serverFixtureKind(t, handler, kind)

			response := request(handler, "POST", "/api/v1/servers", configJSON(t, input))
			if response.Code != http.StatusCreated {
				t.Fatalf("create: %d %s", response.Code, response.Body)
			}

			var server model.Server
			if err := json.Unmarshal(response.Body.Bytes(), &server); err != nil {
				t.Fatal(err)
			}

			endpoint := "/api/v1/servers/" + server.ID + "/configuration"

			response = request(handler, "POST", endpoint, "{}")
			if response.Code != http.StatusOK {
				t.Fatalf("sync: %d %s", response.Code, response.Body)
			}

			var receipt model.ConfigurationReceipt
			if err := json.Unmarshal(response.Body.Bytes(), &receipt); err != nil {
				t.Fatal(err)
			}

			if receipt.ServerID != server.ID || len(receipt.Revision) != 64 {
				t.Fatal("missing configuration acknowledgement")
			}

			data, err := os.ReadFile(filepath.Join(dir, kind, node.ID, server.ID+".json"))
			if err != nil {
				t.Fatal(err)
			}

			var saved model.Deployment
			if err := json.Unmarshal(data, &saved); err != nil {
				t.Fatal(err)
			}

			if !reflect.DeepEqual(saved.Configuration, server.ServerConfiguration) || !reflect.DeepEqual(saved.Ember, ember) || saved.ServerID != server.ID {
				t.Fatal("incomplete delivered definition")
			}

			current, err := servers.Get(context.Background(), server.ID)
			if err != nil || current.Status != "not_installed" {
				t.Fatal("sync changed runtime state")
			}

			response = request(handler, "POST", "/api/v1/servers/missing/configuration", "{}")
			if response.Code != http.StatusNotFound {
				t.Fatalf("unknown server: %d", response.Code)
			}

			request(handler, "PATCH", "/api/v1/sparks/"+node.ID, `{"disabled":true}`)

			response = request(handler, "POST", endpoint, "{}")
			if response.Code != http.StatusServiceUnavailable {
				t.Fatalf("disabled placement: %d", response.Code)
			}

			request(handler, "PATCH", "/api/v1/sparks/"+node.ID, `{"disabled":false}`)

			deployment.Remote = nil

			response = request(handler, "POST", endpoint, "{}")
			if response.Code != http.StatusServiceUnavailable {
				t.Fatalf("unavailable dispatcher: %d", response.Code)
			}
		})
	}
}
