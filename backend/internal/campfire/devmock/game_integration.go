//go:build !production

package devmock

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"campfire/internal/campfire/devfixture"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
)

type gameSession struct {
	id      string
	step    int
	players int
	paused  bool
	next    time.Time
	last    *model.GameEvent
}

// StartGameSimulation connects mock publishers to the real ingestion services.
// Its returned closer waits for the worker before the database is closed.
func (r *Runtime) StartGameSimulation(ctx context.Context, games *service.GameIntegrationService) func() {
	r.mu.Lock()
	r.games = games
	r.gameSessions = make(map[string]*gameSession)
	r.mu.Unlock()

	ctx, cancel := context.WithCancel(ctx)

	done := make(chan struct{})
	go func() {
		defer close(done)

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			if err := r.gameTick(ctx, time.Now()); err != nil && ctx.Err() == nil {
				slog.Warn("Mock game publication", "error", err)
			}

			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	return func() { cancel(); <-done }
}

// gameTick accepts an explicit clock so functional tests never need to sleep.
func (r *Runtime) gameTick(ctx context.Context, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var records []runtimeRecord
	if err := r.db.WithContext(ctx).Find(&records).Error; err != nil {
		return err
	}

	var failures []error

	for _, record := range records {
		if !r.Connected(record.SparkID) || record.State != "running" {
			delete(r.gameSessions, record.ServerID)
			continue
		}

		if err := r.publishGame(ctx, record, now); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", record.ServerID, err))
		}
	}

	return errors.Join(failures...)
}

func (r *Runtime) publishGame(ctx context.Context, record runtimeRecord, now time.Time) error {
	session := r.gameSession(record.ServerID)
	if session.paused {
		return nil
	}

	if !now.Before(session.next) {
		if session.last == nil || session.last.ID != fmt.Sprintf("%s-%d", session.id, session.step) {
			event := model.GameEvent{ID: fmt.Sprintf("%s-%d", session.id, session.step), OccurredAt: now.UTC()}
			switch session.step % 4 {
			case 0:
				event.Type, event.Title = "game.announcement", "Mock game integration connected"
			case 1:
				event.Type, event.Title = "player.joined", "Alex joined the mock server"
				event.Data = map[string]any{"player": "Alex"}
			case 2:
				event.Type, event.Title = "game.announcement", "Welcome to the mock server"
			case 3:
				event.Type, event.Title = "player.left", "Alex left the mock server"
				event.Data = map[string]any{"player": "Alex"}
			}

			session.last = &event
		}

		if err := r.receiveGame(ctx, record, model.GamePublication{Version: 1, Event: session.last}); err != nil {
			return err
		}

		if session.step%4 == 2 && r.games.Media != nil {
			for _, sample := range []struct {
				id, name, contentType string
				bytes                 func() ([]byte, error)
			}{
				{"screenshot", "mock-world.png", "image/png", devfixture.GameScreenshot},
				{"clip", "mock-clip.webm", "video/webm", devfixture.GameClip},
			} {
				data, err := sample.bytes()
				if err != nil {
					return err
				}

				media := model.GameMedia{ID: sample.id, Name: sample.name, ContentType: sample.contentType}
				if err := r.games.Media.Receive(ctx, record.SparkID, record.ServerID, session.last.ID, media, bytes.NewReader(data)); err != nil {
					return err
				}
			}
		}

		if phase := session.step % 4; phase == 1 || phase == 2 {
			session.players = 1
		} else {
			session.players = 0
		}

		session.step++
		session.next = now.Add(15 * time.Second)
	}

	state := &model.GameState{Players: new(session.players), Capacity: new(20)}
	if session.players == 0 {
		state.Fields = map[string]any{"game_mode": "Survival"}
	}

	return r.receiveGame(ctx, record, model.GamePublication{Version: 1, State: state})
}

func (r *Runtime) gameSession(id string) *gameSession {
	if r.gameSessions == nil {
		r.gameSessions = make(map[string]*gameSession)
	}

	if r.gameSessions[id] == nil {
		r.gameSessions[id] = &gameSession{id: fmt.Sprintf("mock-%d", time.Now().UnixNano())}
	}

	return r.gameSessions[id]
}

func (r *Runtime) receiveGame(ctx context.Context, record runtimeRecord, publication model.GamePublication) error {
	return r.games.Receive(ctx, record.SparkID, protocol.GameFrame{ServerID: record.ServerID, Publication: publication})
}

// gameCommand is called under the runtime lock by the normal console path.
func (r *Runtime) gameCommand(ctx context.Context, record runtimeRecord, command string) error {
	if !strings.HasPrefix(command, "mock-game ") || r.games == nil {
		return nil
	}

	session := r.gameSession(record.ServerID)

	switch strings.TrimPrefix(command, "mock-game ") {
	case "pause":
		session.paused = true
	case "resume":
		session.paused = false
	case "next":
		session.next = time.Time{}
	case "replay":
		if session.last != nil {
			return r.receiveGame(ctx, record, model.GamePublication{Version: 1, Event: session.last})
		}
	default:
		return errors.New("mock-game expects pause, resume, next, or replay")
	}

	return nil
}
