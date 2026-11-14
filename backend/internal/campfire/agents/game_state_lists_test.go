package agents

import (
	"context"
	"reflect"
	"testing"

	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
)

func TestGameStateListsRequireNegotiation(t *testing.T) {
	current := &peer{info: AgentInfo{Protocol: protocol.ProtocolVersion, Capabilities: []string{protocol.CapabilityGameIntegration}}, outbound: make(chan protocol.Message, 1), done: make(chan struct{})}
	calls := 0
	hub := (&Hub{}).WithGameIntegrations(func(_ context.Context, _ string, frame protocol.GameFrame) error {
		calls++

		if !reflect.DeepEqual(frame.Publication.State.Fields["player_names"], []any{"Alex"}) {
			t.Fatal("list lost")
		}

		return nil
	})

	frame := &protocol.GameFrame{ID: "request", ServerID: "game", Publication: model.GamePublication{Version: 1, State: &model.GameState{Fields: map[string]any{"player_names": []any{"Alex"}}}}}
	if err := hub.gameFrame(t.Context(), "spark", current, frame); err != nil {
		t.Fatal(err)
	}

	if ack := <-current.outbound; calls != 0 || ack.GameAck.Error == nil || ack.GameAck.Error.Code != protocol.ErrorUnsupportedCapability {
		t.Fatal("unnegotiated list accepted", ack)
	}

	current.info.Capabilities = append(current.info.Capabilities, protocol.CapabilityGameStateLists)
	if err := hub.gameFrame(t.Context(), "spark", current, frame); err != nil {
		t.Fatal(err)
	}

	if ack := <-current.outbound; calls != 1 || ack.GameAck.Error != nil {
		t.Fatal("negotiated list rejected", ack)
	}
}
