package service

import (
	"testing"

	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
)

func TestServerGameIntegrationPersistsAndRevokesPublishing(t *testing.T) {
	f := newTransferFixture(t)
	servers := f.service.Deployment.Servers
	games := &GameIntegrationService{Servers: servers}
	frame := protocol.GameFrame{ServerID: f.server.ID, Publication: model.GamePublication{Version: 1, State: &model.GameState{Players: new(1)}}}
	for _, enabled := range []bool{false, true, false} {
		input := f.server.ServerConfiguration
		input.GameIntegration = enabled
		if _, err := servers.Update(t.Context(), f.server.ID, input); err != nil {
			t.Fatal(err)
		}
		saved, err := servers.Get(t.Context(), f.server.ID)
		if err != nil || saved.GameIntegration != enabled {
			t.Fatal("integration setting not persisted", saved, err)
		}
		err = games.Receive(t.Context(), saved.SparkID, frame)
		if (err == nil) != enabled {
			t.Fatalf("publishing with enabled=%v: %v", enabled, err)
		}
	}
}
