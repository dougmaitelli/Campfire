package agents

import (
	"context"
	"errors"

	"campfire/internal/shared/protocol"
)

// WithGameIntegrations installs the ingestion boundary before serving connections.
// The receiver must validate server placement against authoritative panel records.
func (h *Hub) WithGameIntegrations(receive func(context.Context, string, protocol.GameFrame) error) *Hub {
	h.receiveGame = receive
	return h
}

func (h *Hub) gameFrame(ctx context.Context, sparkID string, current *peer, frame *protocol.GameFrame) error {
	if protocol.Require(current.info.Capabilities, protocol.CapabilityGameIntegration) != nil || frame == nil || frame.ID == "" || len(frame.ID) > 128 {
		return errors.New("invalid game integration frame")
	}

	ack := &protocol.GameAck{ID: frame.ID}
	if frame.Publication.Validate() != nil {
		ack.Error = &protocol.CommandError{Code: "invalid_publication", Message: "Invalid game publication."}
	} else if h.receiveGame == nil {
		ack.Error = &protocol.CommandError{Code: "integration_unavailable", Message: "Game ingestion is unavailable."}
	} else if err := h.receiveGame(ctx, sparkID, *frame); err != nil {
		ack.Error = &protocol.CommandError{Code: "publication_rejected", Message: "Game publication was rejected."}
	}

	select {
	case current.outbound <- protocol.Message{Type: "game.ack", Protocol: current.info.Protocol, GameAck: ack}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-current.done:
		return ErrDisconnected
	default:
		return errors.New("game acknowledgement queue full")
	}
}
