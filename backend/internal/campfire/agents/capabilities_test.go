package agents

import (
	"context"
	"errors"
	"testing"
	"time"

	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
)

func TestCapabilityDispatchFailsBeforeQueueingAndKeepsConnection(t *testing.T) {
	current := &peer{info: AgentInfo{Protocol: protocol.ProtocolVersion, Capabilities: []string{protocol.CapabilityLifecycle}}, pending: make(map[string]chan protocol.CommandResponse), outbound: make(chan protocol.Message, 64), done: make(chan struct{})}
	hub := &Hub{peers: map[string]*peer{"node": current}}
	_, err := hub.Action(context.Background(), "node", model.RuntimeRequest{ServerID: "game", Action: "metrics"})

	var failure *protocol.CommandError
	if !errors.As(err, &failure) || failure.Code != "unsupported_capability" || len(current.pending) != 0 || len(current.outbound) != 0 {
		t.Fatal("unsupported work queued", err)
	}

	if err := hub.Console(context.Background(), "node", "game", func(model.ConsoleEvent) error { return nil }); !errors.As(err, &failure) {
		t.Fatal("console missing-capability error lost", err)
	}

	info := hub.Info("node")

	info.Capabilities[0] = protocol.CapabilityMetrics
	if err := hub.CheckCapabilities("node", protocol.CapabilityMetrics); err == nil {
		t.Fatal("metadata caller changed negotiated support")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	go func() {
		select {
		case message := <-current.outbound:
			current.mu.Lock()
			if response := current.pending[message.Command.ID]; response != nil {
				response <- protocol.CommandResponse{ID: message.Command.ID, Status: &model.RuntimeStatus{ServerID: "game", State: "stopped"}}
			}
			current.mu.Unlock()
		case <-ctx.Done():
		}
	}()

	if _, err := hub.Action(ctx, "node", model.RuntimeRequest{ServerID: "game", Action: "status"}); err != nil {
		t.Fatal("supported command failed after optional feature denial", err)
	}

	if !hub.Connected("node") {
		t.Fatal("optional feature missing disconnected Spark")
	}
}
