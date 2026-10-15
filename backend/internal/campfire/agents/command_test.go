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
