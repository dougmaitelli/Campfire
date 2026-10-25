package service

import (
	"context"
	"errors"
	"testing"
	"time"

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

func TestGameStateReplacementFreshnessAndLifecycle(t *testing.T) {
	now := time.Unix(1000, 0)
	s := &GameIntegrationService{Servers: gameServerStore{}, now: func() time.Time { return now }}
	s.Observe("game", "owner", model.ResourceUsage{State: "running", StartedAt: 900}, now, false)

	frame := protocol.GameFrame{ID: "request", ServerID: "game", Publication: model.GamePublication{Version: 1, State: &model.GameState{Players: new(3), Capacity: new(20), Fields: map[string]any{"map": "world"}}}}
	if err := s.Receive(t.Context(), "owner", frame); err != nil {
		t.Fatal(err)
	}

	*frame.Publication.State.Players = 99

	view := s.Snapshot("game", "owner", true, now.Unix())
	if view.Status != "fresh" || *view.State.Players != 3 || view.State.Fields["map"] != "world" {
		t.Fatal("snapshot not isolated", view)
	}

	view.State.Fields["map"] = "changed"
	if s.Snapshot("game", "owner", true, now.Unix()).State.Fields["map"] != "world" {
		t.Fatal("caller mutated cache")
	}

	if s.Snapshot("game", "different", true, now.Unix()) != nil {
		t.Fatal("state crossed Spark placement")
	}

	if view := s.Snapshot("game", "owner", true, now.Add(60*time.Second).Unix()); view.Status != "stale" || view.State != nil {
		t.Fatal("expired values exposed", view)
	}

	if view := s.Snapshot("game", "owner", false, now.Unix()); view.Status != "unavailable" || view.State != nil {
		t.Fatal("unknown runtime presented current state")
	}

	frame.Publication.State = &model.GameState{Players: new(0)}
	now = now.Add(time.Second)

	if err := s.Receive(t.Context(), "owner", frame); err != nil {
		t.Fatal(err)
	}

	view = s.Snapshot("game", "owner", true, now.Unix())
	if view.State.Capacity != nil || len(view.State.Fields) != 0 || *view.State.Players != 0 {
		t.Fatal("snapshot merged instead of replaced", view)
	}

	before := now

	s.Invalidate("game")

	if view := s.Snapshot("game", "owner", true, now.Unix()); view.State != nil {
		t.Fatal("lifecycle change preserved state")
	}

	if s.Receive(t.Context(), "owner", frame) == nil {
		t.Fatal("publication accepted during transition")
	}

	s.Observe("game", "owner", model.ResourceUsage{State: "running", StartedAt: 900}, before.Add(-time.Second), false)

	if s.Receive(t.Context(), "owner", frame) == nil {
		t.Fatal("in-flight observation reopened state gate")
	}

	now = now.Add(time.Second)
	s.Observe("game", "owner", model.ResourceUsage{State: "running", StartedAt: 900}, now, true)

	if s.Receive(t.Context(), "owner", frame) == nil {
		t.Fatal("busy runtime reopened state gate")
	}

	s.Observe("game", "owner", model.ResourceUsage{State: "running", StartedAt: 900}, now, false)

	if err := s.Receive(t.Context(), "owner", frame); err != nil {
		t.Fatal(err)
	}

	s.Observe("game", "owner", model.ResourceUsage{State: "running", StartedAt: now.Add(time.Second).Unix()}, now.Add(2*time.Second), false)

	if view := s.Snapshot("game", "owner", true, now.Unix()); view.State != nil {
		t.Fatal("external restart preserved state")
	}

	if err := s.Receive(t.Context(), "owner", frame); err != nil {
		t.Fatal(err)
	}

	s.Observe("game", "owner", model.ResourceUsage{State: "stopped"}, now.Add(time.Second), false)

	if view := s.Snapshot("game", "owner", true, now.Unix()); view.State != nil {
		t.Fatal("external stop preserved state")
	}

	s.Retain(nil)

	if s.Snapshot("game", "owner", true, now.Unix()) != nil {
		t.Fatal("deleted server retained state")
	}
}

func TestGameStateUsesDockerClockForRestartDetection(t *testing.T) {
	now := time.Unix(1000, 0)
	s := &GameIntegrationService{Servers: gameServerStore{}, now: func() time.Time { return now }}
	usage := model.ResourceUsage{State: "running", StartedAt: 1000000}
	s.Observe("game", "owner", usage, now, false)

	frame := protocol.GameFrame{ID: "request", ServerID: "game", Publication: model.GamePublication{Version: 1, State: &model.GameState{Players: new(2)}}}
	if err := s.Receive(t.Context(), "owner", frame); err != nil {
		t.Fatal(err)
	}

	s.Observe("game", "owner", usage, now.Add(time.Second), false)

	if s.Snapshot("game", "owner", true, now.Unix()).State == nil {
		t.Fatal("clock offset invalidated a running game's state")
	}

	usage.StartedAt++
	s.Observe("game", "owner", usage, now.Add(2*time.Second), false)

	if s.Snapshot("game", "owner", true, now.Unix()).State != nil {
		t.Fatal("Docker restart failed to invalidate game state")
	}
}

func TestGameEventsDoNotReplaceOrRefreshState(t *testing.T) {
	now := time.Unix(1000, 0)
	s := &GameIntegrationService{Servers: gameServerStore{}, now: func() time.Time { return now }}

	frame := protocol.GameFrame{ID: "request", ServerID: "game", Publication: model.GamePublication{Version: 1, State: &model.GameState{Players: new(2)}}}
	if err := s.Receive(t.Context(), "owner", frame); err != nil {
		t.Fatal(err)
	}

	now = now.Add(61 * time.Second)

	frame.Publication = model.GamePublication{Version: 1, Event: &model.GameEvent{ID: "event", Type: "game.test", Title: "Hello", OccurredAt: now}}
	if err := s.Receive(t.Context(), "owner", frame); err == nil {
		t.Fatal("event acknowledged without durable storage")
	}

	view := s.Snapshot("game", "owner", true, now.Unix())
	if view.Status != "stale" || view.UpdatedAt != 1000 {
		t.Fatal("event refreshed state", view)
	}
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
