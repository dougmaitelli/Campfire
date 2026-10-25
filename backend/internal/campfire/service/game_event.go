package service

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
)

// GameEventService separates retained activity from the latest game snapshot.
type GameEventService struct {
	Store        *repository.GameEventRepository
	Retention    time.Duration
	MaxPerServer int
	cancel       context.CancelFunc
	wg           sync.WaitGroup
}

// NewGameEventService defaults to 30 days and 10,000 events per server.
func NewGameEventService(store *repository.GameEventRepository) *GameEventService {
	return &GameEventService{Store: store, Retention: 30 * 24 * time.Hour, MaxPerServer: 10000}
}

// Validate bounds policy and query work independently of publisher input.
func (s *GameEventService) Validate() error {
	if s.Retention < 24*time.Hour || s.Retention > 3650*24*time.Hour || s.MaxPerServer < 1 || s.MaxPerServer > 100000 {
		return errors.New("game event retention must be 1–3650 days and limit 1–100000 per server")
	}

	return nil
}

// Receive succeeds only after durable storage (including deduplication) commits.
func (s *GameEventService) Receive(ctx context.Context, sparkID, serverID string, event model.GameEvent) error {
	if err := s.Validate(); err != nil {
		return err
	}

	err := s.Store.Append(ctx, sparkID, serverID, event, time.Now(), s.Retention, s.MaxPerServer)
	if errors.Is(err, repository.ErrGameEventPlacement) {
		return &protocol.CommandError{Code: "publication_rejected", Message: "Game event source no longer owns this server."}
	}

	return err
}

// Start prunes at startup and hourly, including quiet servers.
func (s *GameEventService) Start(ctx context.Context) error {
	if err := s.Validate(); err != nil {
		return err
	}

	if err := s.Store.Prune(ctx, time.Now().Add(-s.Retention).Unix(), s.MaxPerServer); err != nil {
		return err
	}

	ctx, s.cancel = context.WithCancel(ctx)

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.Store.Prune(ctx, time.Now().Add(-s.Retention).Unix(), s.MaxPerServer); err != nil && ctx.Err() == nil {
					slog.Error("Prune game events", "error", err)
				}
			}
		}
	}()

	return nil
}

// Close waits for retention work before the database is closed.
func (s *GameEventService) Close() {
	if s.cancel != nil {
		s.cancel()
	}

	s.wg.Wait()
}
