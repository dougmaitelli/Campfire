package agents

import (
	"context"
	"errors"
	"testing"
	"time"

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

func TestGameEventAcknowledgementWaitsForCommitAndClassifiesFailures(t *testing.T) {
	current := &peer{info: AgentInfo{Protocol: protocol.ProtocolVersion, Capabilities: []string{protocol.CapabilityGameIntegration}}, outbound: make(chan protocol.Message, 1), done: make(chan struct{})}
	hub := &Hub{}

	frame := &protocol.GameFrame{ID: "request", ServerID: "game", Publication: model.GamePublication{Version: 1, Event: &model.GameEvent{ID: "event", Type: "game.test", Title: "Hello", OccurredAt: time.Now()}}}
	if err := hub.gameFrame(t.Context(), "node", current, frame); err != nil {
		t.Fatal(err)
	}

	if ack := <-current.outbound; ack.GameAck.Error == nil || ack.GameAck.Error.Code != "unsupported_capability" {
		t.Fatal("older contract acknowledged event durability", ack)
	}

	current.info.Capabilities = append(current.info.Capabilities, protocol.CapabilityGameEvents)
	entered := make(chan struct{})
	commit := make(chan struct{})

	hub.WithGameIntegrations(func(context.Context, string, protocol.GameFrame) error { close(entered); <-commit; return nil })

	finished := make(chan error, 1)
	go func() { finished <- hub.gameFrame(t.Context(), "node", current, frame) }()

	<-entered

	select {
	case <-current.outbound:
		t.Error("event acknowledged before commit")
	default:
	}

	close(commit)

	if err := <-finished; err != nil {
		t.Fatal(err)
	}

	if ack := <-current.outbound; ack.GameAck.Error != nil {
		t.Fatal("committed event was rejected", ack)
	}

	for _, test := range []struct {
		failure error
		code    string
	}{
		{errors.New("storage unavailable"), "ingestion_unavailable"},
		{&protocol.CommandError{Code: "publication_rejected", Message: "Server moved"}, "publication_rejected"},
	} {
		hub.WithGameIntegrations(func(context.Context, string, protocol.GameFrame) error { return test.failure })

		if err := hub.gameFrame(t.Context(), "node", current, frame); err != nil {
			t.Fatal(err)
		}

		if ack := <-current.outbound; ack.GameAck.Error == nil || ack.GameAck.Error.Code != test.code {
			t.Fatal("incorrect retry classification", ack)
		}
	}
}
