package agents

import (
	"context"
	"errors"
	"testing"
	"time"

	"campfire/internal/shared/protocol"
)

func TestCommandTimeoutAndDisconnect(t *testing.T) {
	hub := &Hub{peers: make(map[string]*peer)}
	if _, err := hub.Execute(context.Background(), "absent", protocol.Command{}); !errors.Is(err, ErrDisconnected) {
		t.Fatalf("offline: %v", err)
	}

	current := &peer{pending: make(map[string]chan protocol.CommandResponse), outbound: make(chan protocol.Message, 64), done: make(chan struct{})}
	hub.peers["node"] = current

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	if _, err := hub.Execute(ctx, "node", protocol.Command{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout: %v", err)
	}

	if len(current.pending) != 0 {
		t.Fatal("timed out request leaked")
	}

	finished := make(chan error, 1)

	go func() { _, err := hub.Execute(context.Background(), "node", protocol.Command{}); finished <- err }()

	<-current.outbound
	close(current.done)

	select {
	case err := <-finished:
		if !errors.Is(err, ErrDisconnected) {
			t.Fatalf("disconnect: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("disconnected request blocked")
	}
}

func TestReinstallWaitsForAcknowledgedConnectionToClose(t *testing.T) {
	current := &peer{
		info:    AgentInfo{Protocol: protocol.ProtocolVersion, Capabilities: []string{protocol.CapabilitySelfReinstall}},
		pending: make(map[string]chan protocol.CommandResponse), outbound: make(chan protocol.Message, 1), done: make(chan struct{}),
	}
	hub := &Hub{peers: map[string]*peer{"node": current}}
	finished := make(chan error, 1)

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	go func() { finished <- hub.ReinstallSpark(ctx, "node", "v0.3.0") }()

	var message protocol.Message
	select {
	case message = <-current.outbound:
	case <-ctx.Done():
		t.Fatal("reinstall was not dispatched")
	}

	if message.Command.Name != protocol.ReinstallSpark || message.Command.SparkVersion != "v0.3.0" {
		t.Fatal("wrong reinstall command", message.Command)
	}

	current.mu.Lock()
	response := current.pending[message.Command.ID]
	current.mu.Unlock()

	response <- protocol.CommandResponse{ID: message.Command.ID, SparkVersion: "v0.3.0"}

	select {
	case err := <-finished:
		t.Fatal("acknowledged before the old connection closed", err)
	case <-time.After(20 * time.Millisecond):
	}

	close(current.done)

	if err := <-finished; err != nil {
		t.Fatal(err)
	}
}

func TestReinstallAcknowledgementSurvivesImmediateDisconnect(t *testing.T) {
	for range 100 {
		current := &peer{
			info:    AgentInfo{Protocol: protocol.ProtocolVersion, Capabilities: []string{protocol.CapabilitySelfReinstall}},
			pending: make(map[string]chan protocol.CommandResponse), outbound: make(chan protocol.Message, 1), done: make(chan struct{}),
		}
		hub := &Hub{peers: map[string]*peer{"node": current}}
		finished := make(chan error, 1)

		ctx, cancel := context.WithTimeout(t.Context(), time.Second)

		go func() { finished <- hub.ReinstallSpark(ctx, "node", "v0.3.0") }()

		select {
		case message := <-current.outbound:
			current.mu.Lock()
			response := current.pending[message.Command.ID]
			current.mu.Unlock()

			response <- protocol.CommandResponse{ID: message.Command.ID, SparkVersion: "v0.3.0"}

			close(current.done)
		case <-ctx.Done():
			cancel()
			t.Fatal("reinstall was not dispatched")
		}

		if err := <-finished; err != nil {
			cancel()
			t.Fatal("restart discarded its acknowledgement", err)
		}

		cancel()
	}
}
