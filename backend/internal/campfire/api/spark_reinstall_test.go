package api_test

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"

	"campfire/internal/campfire/api"
	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/protocol"
)

type reinstallConnection struct {
	connected, capable bool
	calls              int
	err                error
}

func (c *reinstallConnection) Connected(string) bool { return c.connected }
func (c *reinstallConnection) SparkFeatures(string) (int, string, []string) {
	features := []string{protocol.CapabilitySelfUpdate}
	if c.capable {
		features = append(features, protocol.CapabilitySelfReinstall)
	}

	return protocol.ProtocolVersion, "v0.3.0", features
}
func (*reinstallConnection) UpdateSpark(context.Context, string, string) error { return nil }
func (c *reinstallConnection) ReinstallSpark(context.Context, string, string) error {
	c.calls++
	return c.err
}

func TestSparkRemoteReinstallDispatchAndErrors(t *testing.T) {
	db, err := database.Open(t.Context(), filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	connection := &reinstallConnection{connected: true, capable: true}
	sparks := service.NewSparkService(repository.NewSparkRepository(db.ORM()), connection)
	sparks.Updates = service.NewSparkUpdateService(sparks, connection).WithFetcher(func(context.Context) (string, error) { return "v0.3.0", nil })
	sparks.Updates.Start(t.Context())

	spark, err := sparks.Create(t.Context(), service.CreateSparkInput{Name: "Node"})
	if err != nil {
		t.Fatal(err)
	}

	embers := service.NewEmberService(repository.NewEmberRepository(db.ORM()))
	servers := service.NewServerService(repository.NewServerRepository(db.ORM()), embers)
	handler := authenticatedTestAPI(t, db, api.New(sparks, embers, servers))
	endpoint := "/api/v1/sparks/" + spark.ID + "/update"

	response := request(handler, "POST", endpoint, `{"version":"v0.3.0","force":true}`)
	if response.Code != http.StatusAccepted || connection.calls != 1 {
		t.Fatal("same-version reinstall failed", response.Code, response.Body)
	}

	response = request(handler, "POST", endpoint, `{"version":"v0.3.0"}`)
	if response.Code != http.StatusConflict || connection.calls != 1 {
		t.Fatal("ordinary update bypassed the version check", response.Code, response.Body)
	}

	for _, scenario := range []struct {
		connected, capable bool
		body               string
	}{
		{false, true, `{"version":"v0.3.0","force":true}`},
		{true, false, `{"version":"v0.3.0","force":true}`},
		{true, true, `{"version":"v0.4.0","force":true}`},
	} {
		connection.connected, connection.capable = scenario.connected, scenario.capable

		response = request(handler, "POST", endpoint, scenario.body)
		if response.Code != http.StatusConflict || connection.calls != 1 {
			t.Fatal("invalid reinstall dispatched", scenario, response.Code, response.Body)
		}
	}

	connection.connected, connection.capable = true, true
	connection.err = &protocol.CommandError{Code: "spark_update_failed", Message: "Wait for active server operations to finish."}

	response = request(handler, "POST", endpoint, `{"version":"v0.3.0","force":true}`)
	if response.Code != http.StatusConflict || connection.calls != 2 {
		t.Fatal("reinstall failure was not actionable", response.Code, response.Body)
	}
}
