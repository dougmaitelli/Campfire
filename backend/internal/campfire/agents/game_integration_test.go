package agents

import (
	"context"
	"errors"
	"testing"

	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
)

func TestGameFrameCapabilityAndAcknowledgement(t *testing.T) {
	current := &peer{info: AgentInfo{Protocol: protocol.ProtocolVersion}, outbound: make(chan protocol.Message, 1), done: make(chan struct{})}
	hub := &Hub{}

	frame := &protocol.GameFrame{ID: "request", ServerID: "game", Publication: model.GamePublication{Version: 1, State: &model.GameState{}}}
	if hub.gameFrame(context.Background(), "spark", current, frame) == nil {
		t.Fatal("unnegotiated publication accepted")
	}

	current.info.Capabilities = []string{protocol.CapabilityGameIntegration}

	for _, test := range []struct{ reject bool }{{false}, {true}} {
		hub.WithGameIntegrations(func(_ context.Context, sparkID string, received protocol.GameFrame) error {
			if sparkID != "spark" || received.ServerID != "game" {
				t.Fatal("source identity lost")
			}

			if test.reject {
				return errors.New("placement rejected")
			}

			return nil
		})

		if err := hub.gameFrame(context.Background(), "spark", current, frame); err != nil {
			t.Fatal(err)
		}

		ack := <-current.outbound
		if ack.Type != "game.ack" || ack.GameAck.ID != frame.ID || (ack.GameAck.Error != nil) != test.reject {
			t.Fatal("incorrect ingestion acknowledgement", ack)
		}
	}
}
