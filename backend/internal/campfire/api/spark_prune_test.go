package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"campfire/internal/campfire/api"
	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
)

type pruneConnection struct {
	connected bool
	calls     int
	err       error
}

func (c *pruneConnection) Connected(string) bool { return c.connected }
func (c *pruneConnection) PruneSpark(context.Context, string) (model.PruneResult, error) {
	c.calls++
	return model.PruneResult{Containers: 2, Images: 3, Volumes: 1}, c.err
}

func TestSparkPruneDispatchAndErrors(t *testing.T) {
	db, err := database.Open(t.Context(), filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	connection := &pruneConnection{connected: true}
	sparks := service.NewSparkService(repository.NewSparkRepository(db.ORM()), connection)

	spark, err := sparks.Create(t.Context(), service.CreateSparkInput{Name: "Node"})
	if err != nil {
		t.Fatal(err)
	}

	embers := service.NewEmberService(repository.NewEmberRepository(db.ORM()))
	servers := service.NewServerService(repository.NewServerRepository(db.ORM()), embers)
	handler := authenticatedTestAPI(t, db, api.New(sparks, embers, servers))
	endpoint := "/api/v1/sparks/" + spark.ID + "/prune"
	response := request(handler, "POST", endpoint, "{}")

	var result model.PruneResult
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &result) != nil || result.Images != 3 || connection.calls != 1 {
		t.Fatal("cleanup response", response.Code, response.Body)
	}

	connection.connected = false

	response = request(handler, "POST", endpoint, "{}")
	if response.Code != http.StatusConflict || connection.calls != 1 {
		t.Fatal("offline node accepted cleanup", response.Code, response.Body)
	}

	connection.connected = true
	connection.err = &protocol.CommandError{Code: "spark_prune_failed", Message: "Wait for active server operations to finish."}

	response = request(handler, "POST", endpoint, "{}")
	if response.Code != http.StatusConflict {
		t.Fatal("cleanup failure was not actionable", response.Code, response.Body)
	}
}
