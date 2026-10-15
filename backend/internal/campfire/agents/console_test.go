package agents

import (
	"testing"

	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
)

func TestConsoleRoutingAndSlowConsumer(t *testing.T) {
	sub := &consoleSubscription{server: "game", events: make(chan model.ConsoleEvent, 1), overflow: make(chan struct{})}
	p := &peer{consoles: map[string]*consoleSubscription{"stream": sub}}

	frame := &protocol.ConsoleFrame{ID: "stream", ServerID: "other", Event: model.ConsoleEvent{Content: "private"}}
	if p.consoleFrame(frame) {
		t.Fatal("cross-server output accepted")
	}

	frame.ServerID = "game"
	if !p.consoleFrame(frame) {
		t.Fatal("valid frame rejected")
	}

	if !p.consoleFrame(frame) {
		t.Fatal("slow consumer closed agent connection")
	}

	select {
	case <-sub.overflow:
	default:
		t.Fatal("overflow was silently dropped")
	}

	if len(sub.events) != 1 {
		t.Fatal("unbounded stream buffer")
	}

	frame.ID = "cancelled"
	if !p.consoleFrame(frame) {
		t.Fatal("late frame after cancellation rejected")
	}

	frame.ID = "stream"

	frame.Event.Content = string(make([]byte, 8193))
	if p.consoleFrame(frame) {
		t.Fatal("oversized output accepted")
	}
}
