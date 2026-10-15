package agents

import (
	"context"
	"crypto/rand"
	"errors"
	"time"

	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
)

type consoleSubscription struct {
	server   string
	events   chan model.ConsoleEvent
	overflow chan struct{}
}

func (p *peer) consoleFrame(frame *protocol.ConsoleFrame) bool {
	if frame == nil || frame.ID == "" || len(frame.ID) > 128 || len(frame.Event.Content) > 8192 || len(frame.Event.Error) > 256 {
		return false
	}

	if frame.Event.Ready {
		if frame.Event.End || frame.Event.Content != "" || frame.Event.Error != "" {
			return false
		}
	} else if frame.Event.End {
		if frame.Event.Content != "" {
			return false
		}
	} else if frame.Event.Content == "" || frame.Event.Error != "" {
		return false
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	sub := p.consoles[frame.ID]
	if sub == nil {
		return true
	}

	if frame.ServerID != sub.server {
		return false
	}

	select {
	case <-sub.overflow:
		return true
	default:
	}

	select {
	case sub.events <- frame.Event:
	default:
		close(sub.overflow)
	}

	return true
}

// Console streams only from the currently authenticated Spark session.
func (h *Hub) Console(ctx context.Context, node, server string, emit func(model.ConsoleEvent) error) error {
	h.mu.RLock()
	p := h.peers[node]
	h.mu.RUnlock()

	if p == nil {
		return ErrDisconnected
	}

	id := rand.Text()
	sub := &consoleSubscription{server: server, events: make(chan model.ConsoleEvent, 16), overflow: make(chan struct{})}

	p.mu.Lock()
	if p.consoles == nil {
		p.consoles = make(map[string]*consoleSubscription)
	}

	if len(p.consoles) >= 16 {
		p.mu.Unlock()
		return errors.New("spark console stream limit reached")
	}

	p.consoles[id] = sub
	p.mu.Unlock()

	frame := &protocol.ConsoleFrame{ID: id, ServerID: server}

	defer func() {
		p.mu.Lock()
		delete(p.consoles, id)
		p.mu.Unlock()
		// Failure to enqueue cancellation closes this session rather than leaking a producer.
		select {
		case p.outbound <- protocol.Message{Type: "console.close", Protocol: protocol.ProtocolVersion, Console: frame}:
		case <-p.done:
		default:
			_ = p.conn.Close()
		}
	}()

	select {
	case p.outbound <- protocol.Message{Type: "console.open", Protocol: protocol.ProtocolVersion, Console: frame}:
	case <-ctx.Done():
		return ctx.Err()
	case <-p.done:
		return ErrDisconnected
	}

	startup := time.NewTimer(10 * time.Second)
	defer startup.Stop()

	ready := false

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-p.done:
			return ErrDisconnected
		case <-sub.overflow:
			return errors.New("console consumer too slow; reconnect required")
		case <-startup.C:
			return errors.New("console did not open")
		case event := <-sub.events:
			if !ready {
				if !event.Ready && !event.End {
					return errors.New("console output before acknowledgement")
				}

				ready = true

				startup.Stop()
			}

			if err := emit(event); err != nil {
				return err
			}

			if event.End {
				return nil
			}

			select {
			case p.outbound <- protocol.Message{Type: "console.ack", Protocol: protocol.ProtocolVersion, Console: frame}:
			case <-ctx.Done():
				return ctx.Err()
			case <-p.done:
				return ErrDisconnected
			}
		}
	}
}
