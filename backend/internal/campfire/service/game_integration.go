package service

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"sync"
	"time"

	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
)

// GameIntegrationService validates ingestion against current server placement.
// Only the latest state is retained in memory; events are not stored here.
type GameIntegrationService struct {
	Servers interface {
		Get(context.Context, string) (model.Server, error)
	}
	Events  *GameEventService
	Media   *GameMediaService
	mu      sync.Mutex
	states  map[string]gameStateRecord
	blocked map[string]time.Time
	started map[string]gameRuntimeStart
	now     func() time.Time
}

type gameRuntimeStart struct {
	sparkID string
	at      int64
}

const gameStateFreshFor = 60 * time.Second

type gameStateRecord struct {
	sparkID  string
	received time.Time
	state    *model.GameState
}

// GameStateView deliberately omits expired or invalidated values.
type GameStateView struct {
	State           *model.GameState `json:"state,omitempty"`
	UpdatedAt       int64            `json:"updated_at"`
	FreshForSeconds int64            `json:"fresh_for_seconds"`
	Status          string           `json:"status"`
}

func (s *GameIntegrationService) clock() time.Time {
	if s.now != nil {
		return s.now()
	}

	return time.Now()
}

func cloneGameState(state *model.GameState) *model.GameState {
	if state == nil {
		return nil
	}

	result := *state

	result.Fields = maps.Clone(state.Fields)
	if state.Players != nil {
		result.Players = new(*state.Players)
	}

	if state.Capacity != nil {
		result.Capacity = new(*state.Capacity)
	}

	return &result
}

// Receive accepts publications only from the server's currently assigned Spark.
func (s *GameIntegrationService) Receive(ctx context.Context, sparkID string, frame protocol.GameFrame) error {
	if err := frame.Publication.Validate(); err != nil {
		return err
	}

	server, err := s.Servers.Get(ctx, frame.ServerID)
	if errors.Is(err, ErrServerNotFound) || err == nil && (server.SparkID != sparkID || !server.GameIntegration) {
		return &protocol.CommandError{Code: "publication_rejected", Message: "Invalid game publication placement."}
	}

	if err != nil {
		return err
	}

	kind := "state"
	if frame.Publication.Event != nil {
		kind = "event"

		if s.Events == nil {
			return errors.New("durable game events unavailable")
		}

		if err := s.Events.Receive(ctx, sparkID, frame.ServerID, *frame.Publication.Event); err != nil {
			return err
		}
	} else {
		s.mu.Lock()
		if _, blocked := s.blocked[frame.ServerID]; blocked {
			s.mu.Unlock()
			return errors.New("game state unavailable during runtime transition")
		}

		if s.states == nil {
			s.states = make(map[string]gameStateRecord)
		}

		s.states[frame.ServerID] = gameStateRecord{sparkID: sparkID, received: s.clock(), state: cloneGameState(frame.Publication.State)}
		s.mu.Unlock()
	}

	slog.Debug("Game publication accepted", "spark", sparkID, "server", frame.ServerID, "kind", kind)

	return nil
}

// Invalidate closes the state gate before a lifecycle change. A subsequent
// non-busy running observation must reopen it; in-flight old samples cannot.
func (s *GameIntegrationService) Invalidate(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.blocked == nil {
		s.blocked = make(map[string]time.Time)
	}

	s.blocked[id] = s.clock()
	if record, ok := s.states[id]; ok {
		record.state = nil
		s.states[id] = record
	}
}

// Observe uses the existing runtime sampler to detect external stops/restarts.
func (s *GameIntegrationService) Observe(id, sparkID string, usage model.ResourceUsage, began time.Time, busy bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if barrier, blocked := s.blocked[id]; blocked && began.Before(barrier) {
		return
	}

	if busy {
		return
	}

	if usage.State == "running" && usage.ContainerError == "" {
		delete(s.blocked, id)
	}

	previous := s.started[id]
	if usage.StartedAt > 0 {
		if s.started == nil {
			s.started = make(map[string]gameRuntimeStart)
		}

		s.started[id] = gameRuntimeStart{sparkID: sparkID, at: usage.StartedAt}
	}

	record, ok := s.states[id]
	if !ok || record.sparkID != sparkID {
		return
	}

	// Compare Docker timestamps with prior Docker timestamps, never the panel's
	// receive clock: Sparks and Campfire may have different clock offsets.
	restarted := previous.sparkID == sparkID && previous.at > 0 && usage.StartedAt > 0 && previous.at != usage.StartedAt
	if usage.State != "running" || restarted {
		record.state = nil
		s.states[id] = record
	}
}

// Snapshot never returns old game values as current when runtime data is unknown.
func (s *GameIntegrationService) Snapshot(id, sparkID string, runtimeFresh bool, now int64) *GameStateView {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.states[id]
	if !ok || record.sparkID != sparkID {
		return nil
	}

	view := &GameStateView{UpdatedAt: record.received.Unix(), FreshForSeconds: int64(gameStateFreshFor.Seconds()), Status: "unavailable"}

	_, blocked := s.blocked[id]
	if record.state == nil || blocked || !runtimeFresh {
		return view
	}

	if now-record.received.Unix() >= view.FreshForSeconds {
		view.Status = "stale"
		return view
	}

	view.State, view.Status = cloneGameState(record.state), "fresh"

	return view
}

// Retain drops records for deleted servers without retaining historical state.
func (s *GameIntegrationService) Retain(targets []model.ResourceTarget) {
	s.mu.Lock()
	defer s.mu.Unlock()

	keep := make(map[string]bool, len(targets))
	for _, target := range targets {
		keep[target.ID] = true
	}

	for id := range s.states {
		if !keep[id] {
			delete(s.states, id)
		}
	}

	for id := range s.blocked {
		if !keep[id] {
			delete(s.blocked, id)
		}
	}

	for id := range s.started {
		if !keep[id] {
			delete(s.started, id)
		}
	}
}
