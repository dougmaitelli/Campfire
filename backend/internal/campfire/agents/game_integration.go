package agents

import (
	"context"
	"errors"
	"time"

	"campfire/internal/shared/protocol"
)

// WithGameIntegrations installs the ingestion boundary before serving connections.
// The receiver must validate server placement and commit event storage before returning.
func (h *Hub) WithGameIntegrations(receive func(context.Context, string, protocol.GameFrame) error) *Hub {
	h.receiveGame = receive
	return h
}

func (h *Hub) gameFrame(ctx context.Context, sparkID string, current *peer, frame *protocol.GameFrame) error {
	if protocol.Require(current.info.Capabilities, protocol.CapabilityGameIntegration) != nil || frame == nil || frame.ID == "" || len(frame.ID) > 128 || frame.ServerID == "" || len(frame.ServerID) > 128 {
		return errors.New("invalid game integration frame")
	}

	receiveCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	ack := &protocol.GameAck{ID: frame.ID}
	if frame.Publication.Event != nil && protocol.Require(current.info.Capabilities, protocol.CapabilityGameEvents) != nil {
		ack.Error = &protocol.CommandError{Code: "unsupported_capability", Message: "Durable game events were not negotiated."}
	} else if frame.Publication.Validate() != nil {
		ack.Error = &protocol.CommandError{Code: "invalid_publication", Message: "Invalid game publication."}
	} else if h.receiveGame == nil {
		ack.Error = &protocol.CommandError{Code: "integration_unavailable", Message: "Game ingestion is unavailable."}
	} else if err := h.receiveGame(receiveCtx, sparkID, *frame); err != nil {
		var rejection *protocol.CommandError
		if errors.As(err, &rejection) && rejection.Code == "publication_rejected" {
			ack.Error = &protocol.CommandError{Code: "publication_rejected", Message: "Game publication was rejected."}
		} else {
			ack.Error = &protocol.CommandError{Code: "ingestion_unavailable", Message: "Game ingestion is temporarily unavailable."}
		}
	}

	select {
	case current.outbound <- protocol.Message{Type: protocol.MessageTypeGameAck, Protocol: current.info.Protocol, GameAck: ack}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-current.done:
		return ErrDisconnected
	default:
		return errors.New("game acknowledgement queue full")
	}
}
