package agents

import (
	"context"
	"testing"

	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
)

func TestRegistrationRequiresNegotiationAndCommit(t *testing.T) {
	current := &peer{info: AgentInfo{Protocol: protocol.ProtocolVersion, Capabilities: []string{protocol.CapabilityGameIntegration}}, outbound: make(chan protocol.Message, 1), done: make(chan struct{})}
	hub := &Hub{}

	frame := &protocol.GameFrame{ID: "registration", ServerID: "game", Publication: model.GamePublication{Version: 1, Registration: &model.GameRegistration{Revision: 1}}}
	if err := hub.gameFrame(t.Context(), "node", current, frame); err != nil {
		t.Fatal(err)
	}

	if ack := <-current.outbound; ack.GameAck.Error == nil || ack.GameAck.Error.Code != protocol.ErrorUnsupportedCapability {
		t.Fatal("registration accepted without negotiation", ack)
	}

	current.info.Capabilities = append(current.info.Capabilities, protocol.CapabilityGameRegistration)
	entered, commit := make(chan struct{}), make(chan struct{})

	hub.WithGameIntegrations(func(context.Context, string, protocol.GameFrame) error { close(entered); <-commit; return nil })

	finished := make(chan error, 1)
	go func() { finished <- hub.gameFrame(t.Context(), "node", current, frame) }()

	<-entered

	select {
	case <-current.outbound:
		t.Error("registration acknowledged before commit")
	default:
	}

	close(commit)

	if err := <-finished; err != nil {
		t.Fatal(err)
	}

	if ack := <-current.outbound; ack.GameAck.Error != nil {
		t.Fatal("registration commit failed", ack)
	}
}
