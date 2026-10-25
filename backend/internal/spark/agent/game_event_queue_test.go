package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"campfire/internal/shared/agentidentity"
	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
)

func queuedPublication(id string) model.GamePublication {
	return model.GamePublication{Version: 1, Event: &model.GameEvent{ID: id, Type: "game.announcement", Title: "Hello", OccurredAt: time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)}}
}

func TestGameEventQueueSurvivesRestartAndRetriesLostAcknowledgements(t *testing.T) {
	directory := t.TempDir()
	create := func() *gameIntegrations {
		g, err := newGameIntegrations(t.Context(), directory, func(string) bool { return true })
		if err != nil {
			t.Fatal(err)
		}

		return g
	}
	g := create()

	publication := queuedPublication("one")
	if err := g.enqueueEvent("game", publication); err != nil {
		t.Fatal(err)
	}

	if err := g.enqueueEvent("game", publication); err != nil {
		t.Fatal(err)
	}

	changed := queuedPublication("one")

	changed.Event.Title = "Different content"
	if !errors.Is(g.enqueueEvent("game", changed), errGameEventConflict) {
		t.Fatal("conflicting queued ID accepted")
	}

	g.close()

	g = create()
	defer g.close()

	var deliveries atomic.Int32

	old, cancel := context.WithCancel(t.Context())
	g.attach(old, func(_ protocol.Message) error { deliveries.Add(1); cancel(); return nil }, true)
	waitCondition(t, func() bool { return deliveries.Load() == 1 })
	g.mu.Lock()
	if len(g.events) != 1 {
		t.Error("unacknowledged event removed")
	}
	g.mu.Unlock()
	g.attach(t.Context(), func(message protocol.Message) error {
		if message.Game.Publication.Event.ID != "one" || message.Game.ServerID != "game" {
			return errors.New("source or stable event ID changed")
		}

		deliveries.Add(1)

		return g.acknowledge(&protocol.GameAck{ID: message.Game.ID})
	}, true)
	waitCondition(t, func() bool { g.mu.Lock(); defer g.mu.Unlock(); return len(g.events) == 0 })

	if deliveries.Load() != 2 {
		t.Fatal("event was not retried", deliveries.Load())
	}

	if _, err := os.Stat(g.eventPath(gameEventKey("game", "one"))); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("acknowledged event remained on disk", err)
	}
}

func TestGameEventQueueBackpressureAndCapability(t *testing.T) {
	g, err := newGameIntegrations(t.Context(), t.TempDir(), func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	defer g.close()
	// Fill the in-memory index of the bounded spool without creating 512 fsyncs.
	g.mu.Lock()
	for i := range gameQueuePerServer {
		g.events[fmt.Sprint(i)] = &queuedGameEvent{ServerID: "full", Publication: queuedPublication(fmt.Sprint(i)), QueuedAt: time.Now()}
	}
	g.mu.Unlock()

	if !errors.Is(g.enqueueEvent("full", queuedPublication("overflow")), errGameQueueFull) {
		t.Fatal("queue overflow silently accepted")
	}

	other := queuedPublication("other")
	if err := g.enqueueEvent("other", other); err != nil {
		t.Fatal("one server exhausted another's queue", err)
	}

	g.attach(t.Context(), func(protocol.Message) error { return errors.New("must not send to older panel") })

	if g.enqueueEvent("other", queuedPublication("unsupported")) == nil {
		t.Fatal("older panel accepted durable event")
	}

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/events/queue", nil)
	response := httptest.NewRecorder()
	g.queueStatus(response, request, "other")

	var status struct {
		Pending int `json:"pending"`
	}
	if json.Unmarshal(response.Body.Bytes(), &status) != nil || status.Pending != 1 {
		t.Fatal("queue status leaked another server or lost pending event", response.Body)
	}
}

func TestGameEventQueueHTTPAcceptanceAndPermanentRejection(t *testing.T) {
	g, err := newGameIntegrations(t.Context(), t.TempDir(), func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	defer g.close()

	data, _ := json.Marshal(queuedPublication("pending"))
	response := httptest.NewRecorder()
	g.publish(response, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/publish", strings.NewReader(string(data))), "game")

	if response.Code != http.StatusAccepted {
		t.Fatal("offline event was not durably queued", response.Code, response.Body)
	}

	g.attach(t.Context(), func(message protocol.Message) error {
		return g.acknowledge(&protocol.GameAck{ID: message.Game.ID, Error: &protocol.CommandError{Code: "publication_rejected", Message: "Server moved"}})
	}, true)
	waitCondition(t, func() bool { g.mu.Lock(); defer g.mu.Unlock(); return g.dropped["game"] == 1 && len(g.events) == 0 })

	if _, err := os.Stat(filepath.Join(g.directory, "queue")); err != nil {
		t.Fatal(err)
	}
}

func TestGameEventQueueExpiryAndCorruptionPreserveExplicitOutcomes(t *testing.T) {
	directory := t.TempDir()

	g, err := newGameIntegrations(t.Context(), directory, func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}

	if err := g.enqueueEvent("game", queuedPublication("expired")); err != nil {
		t.Fatal(err)
	}

	g.close()
	path := g.eventPath(gameEventKey("game", "expired"))

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var item queuedGameEvent
	if err := json.Unmarshal(data, &item); err != nil {
		t.Fatal(err)
	}

	item.QueuedAt = time.Now().Add(-25 * time.Hour)

	data, err = json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}

	if err := agentidentity.WritePrivate(path, data); err != nil {
		t.Fatal(err)
	}

	g, err = newGameIntegrations(t.Context(), directory, func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}

	waitCondition(t, func() bool { g.mu.Lock(); defer g.mu.Unlock(); return g.dropped["game"] == 1 && len(g.events) == 0 })
	g.close()

	corrupt := filepath.Join(directory, "queue", "corrupt.json")
	if err := agentidentity.WritePrivate(corrupt, []byte("{}")); err != nil {
		t.Fatal(err)
	}

	if restored, err := newGameIntegrations(t.Context(), directory, func(string) bool { return true }); err == nil {
		restored.close()
		t.Fatal("corrupt spool silently accepted")
	}

	if _, err := os.Stat(corrupt); err != nil {
		t.Fatal("corrupt spool was silently deleted", err)
	}
}

func TestGameEventQueueDoesNotAcceptFailedPersistence(t *testing.T) {
	g, err := newGameIntegrations(t.Context(), t.TempDir(), func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	defer g.close()

	path := g.eventPath(gameEventKey("game", "blocked"))
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}

	data, _ := json.Marshal(queuedPublication("blocked"))
	response := httptest.NewRecorder()
	g.publish(response, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/publish", strings.NewReader(string(data))), "game")

	if response.Code != http.StatusServiceUnavailable {
		t.Fatal("event accepted without durable storage", response.Code)
	}

	g.mu.Lock()
	count := len(g.events)
	g.mu.Unlock()

	if count != 0 {
		t.Fatal("failed event entered delivery index")
	}

	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		t.Fatal("unexpected queue path was removed", err)
	}
}
