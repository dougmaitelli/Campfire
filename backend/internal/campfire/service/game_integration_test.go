package service

import (
	"context"
	"errors"
	"testing"

	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
)

type gameServerStore struct{}

func (gameServerStore) Get(_ context.Context, id string) (model.Server, error) {
	if id != "game" {
		return model.Server{}, errors.New("missing")
	}

	return model.Server{ID: id, ServerConfiguration: model.ServerConfiguration{SparkID: "owner"}}, nil
}

func TestGameIngestionValidatesCurrentPlacement(t *testing.T) {
	s := &GameIntegrationService{Servers: gameServerStore{}}

	frame := protocol.GameFrame{ID: "request", ServerID: "game", Publication: model.GamePublication{Version: 1, State: &model.GameState{}}}
	if err := s.Receive(context.Background(), "owner", frame); err != nil {
		t.Fatal(err)
	}

	if s.Receive(context.Background(), "other", frame) == nil {
		t.Fatal("wrong Spark accepted")
	}

	frame.ServerID = "missing"
	if s.Receive(context.Background(), "owner", frame) == nil {
		t.Fatal("missing server accepted")
	}

	frame.ServerID = "game"

	frame.Publication.Version = 2
	if s.Receive(context.Background(), "owner", frame) == nil {
		t.Fatal("invalid version accepted")
	}
}
