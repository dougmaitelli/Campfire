package service

import (
	"context"
	"errors"
	"log/slog"

	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
)

// GameIntegrationService validates ingestion against current server placement.
// State projection and durable event history are later implementation stages.
type GameIntegrationService struct {
	Servers interface {
		Get(context.Context, string) (model.Server, error)
	}
}

// Receive accepts publications only from the server's currently assigned Spark.
func (s *GameIntegrationService) Receive(ctx context.Context, sparkID string, frame protocol.GameFrame) error {
	if err := frame.Publication.Validate(); err != nil {
		return err
	}

	server, err := s.Servers.Get(ctx, frame.ServerID)
	if err != nil || server.SparkID != sparkID {
		return errors.New("invalid game publication placement")
	}

	kind := "state"
	if frame.Publication.Event != nil {
		kind = "event"
	}

	slog.Debug("Game publication accepted", "spark", sparkID, "server", frame.ServerID, "kind", kind)

	return nil
}
