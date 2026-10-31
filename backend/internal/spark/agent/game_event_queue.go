package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"campfire/internal/shared/agentidentity"
	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
	"campfire/internal/shared/utils"
)

const gameQueueMaximum = 4096
const gameQueuePerServer = 512
const gameQueueLifetime = 24 * time.Hour

var errGameQueueFull = errors.New("game event queue is full; retry later with the same event ID")
var errGameEventConflict = errors.New("event ID is already queued with different content")

type queuedGameEvent struct {
	ServerID    string                `json:"server_id"`
	Publication model.GamePublication `json:"publication"`
	QueuedAt    time.Time             `json:"queued_at"`
	attempts    int
	next        time.Time
}

func gameEventKey(serverID, eventID string) string {
	hash := sha256.Sum256([]byte(serverID + "\x00" + eventID))
	return hex.EncodeToString(hash[:])
}

func (g *gameIntegrations) eventPath(key string) string {
	return filepath.Join(g.directory, "queue", key+".json")
}

func (g *gameIntegrations) loadEvents() error {
	directory := filepath.Join(g.directory, "queue")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}

	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}

	counts := map[string]int{}

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".identity-") {
			continue
		}

		path := filepath.Join(directory, entry.Name())

		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > model.GamePublicationLimit+1024 {
			return errors.New("invalid persisted game event queue file")
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		var item queuedGameEvent
		if json.Unmarshal(data, &item) != nil || !utils.IsValidResourceID(item.ServerID) || item.Publication.Event == nil || item.Publication.Validate() != nil || item.QueuedAt.IsZero() {
			return errors.New("invalid persisted game event")
		}

		key := gameEventKey(item.ServerID, item.Publication.Event.ID)
		if entry.Name() != key+".json" {
			return errors.New("invalid persisted game event identity")
		}

		counts[item.ServerID]++
		if len(g.events) >= gameQueueMaximum || counts[item.ServerID] > gameQueuePerServer {
			return errors.New("persisted game event queue exceeds capacity")
		}

		g.events[key] = &item
	}

	return nil
}

func (g *gameIntegrations) enqueueEvent(serverID string, publication model.GamePublication) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.closed || g.events == nil {
		return errors.New("game event queue unavailable")
	}

	if g.session != nil && g.session.ctx.Err() == nil && !g.session.events {
		return errors.New("durable game events require an updated Campfire")
	}

	key := gameEventKey(serverID, publication.Event.ID)
	if existing := g.events[key]; existing != nil {
		previous, _ := json.Marshal(existing.Publication)

		current, _ := json.Marshal(publication)
		if !bytes.Equal(previous, current) {
			return errGameEventConflict
		}

		return nil
	}

	count := 0

	for _, item := range g.events {
		if item.ServerID == serverID {
			count++
		}
	}

	if len(g.events) >= gameQueueMaximum || count >= gameQueuePerServer {
		return errGameQueueFull
	}

	item := &queuedGameEvent{ServerID: serverID, Publication: publication, QueuedAt: time.Now().UTC()}

	data, err := json.Marshal(item)
	if err != nil {
		return err
	}

	var retained queuedGameEvent
	if err := json.Unmarshal(data, &retained); err != nil {
		return err
	}

	if err := agentidentity.WritePrivate(g.eventPath(key), data); err != nil {
		// A failed directory sync may follow a successful rename. Remove only
		// our regular file so rejected requests cannot grow an unindexed spool.
		if info, statErr := os.Lstat(g.eventPath(key)); statErr == nil && info.Mode().IsRegular() {
			_ = os.Remove(g.eventPath(key))
		}

		return errors.New("could not persist game event; retry with the same ID")
	}

	g.events[key] = &retained
	select {
	case g.wake <- struct{}{}:
	default:
	}

	return nil
}

func (g *gameIntegrations) runEvents() {
	defer g.wg.Done()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-g.ctx.Done():
			return
		case <-g.wake:
		case <-ticker.C:
		}

		g.deliverEvents()
	}
}

func (g *gameIntegrations) deliverEvents() {
	g.mu.Lock()

	keys := make([]string, 0, len(g.events))
	for key := range g.events {
		keys = append(keys, key)
	}

	sort.Slice(keys, func(i, j int) bool { return g.events[keys[i]].QueuedAt.Before(g.events[keys[j]].QueuedAt) })
	g.mu.Unlock()

	processed := 0

	for _, key := range keys {
		if g.ctx.Err() != nil {
			return
		}

		g.mu.Lock()
		item := g.events[key]
		session := g.session
		connected := session != nil && session.ctx.Err() == nil && session.events

		next := time.Time{}
		if item != nil {
			next = item.next
		}
		g.mu.Unlock()

		if item == nil {
			continue
		}

		if time.Since(item.QueuedAt) >= gameQueueLifetime || !g.authorize(item.ServerID) {
			g.finishEvent(key, item, "Event expired or local server placement changed.")
			continue
		}

		if !connected || time.Now().Before(next) {
			continue
		}

		ctx, cancel := context.WithTimeout(g.ctx, 8*time.Second)
		err := g.forward(ctx, item.ServerID, item.Publication)

		cancel()

		var rejection *protocol.CommandError
		if err == nil {
			g.finishEvent(key, item, "")
		} else if errors.As(err, &rejection) && (rejection.Code == "publication_rejected" || rejection.Code == "invalid_publication") {
			g.finishEvent(key, item, "Campfire permanently rejected the event.")
		} else {
			g.mu.Lock()
			if g.session == session {
				item.attempts++
				item.next = time.Now().Add(time.Duration(min(1<<min(item.attempts, 5), 30)) * time.Second)
			}

			g.failures[item.ServerID] = "Waiting for durable acknowledgement from Campfire."
			g.mu.Unlock()
		}

		processed++
		if processed >= 32 {
			return
		}
	}
}

func (g *gameIntegrations) finishEvent(key string, item *queuedGameEvent, reason string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if err := os.Remove(g.eventPath(key)); err != nil && !errors.Is(err, os.ErrNotExist) {
		g.failures[item.ServerID] = "Could not remove acknowledged queue entry."
		return
	}

	directory, err := os.Open(filepath.Dir(g.eventPath(key)))
	if err == nil {
		err = directory.Sync()
		_ = directory.Close()
	}

	if err != nil {
		g.failures[item.ServerID] = "Could not synchronize queue removal."
		return
	}

	delete(g.events, key)

	g.failures[item.ServerID] = reason
	if reason != "" {
		g.dropped[item.ServerID]++
		slog.Warn("Game event dropped", "server", item.ServerID, "event", item.Publication.Event.ID, "reason", reason)
	}
}

func (g *gameIntegrations) queueStatus(w http.ResponseWriter, _ *http.Request, id string) {
	if !g.authorize(id) {
		http.Error(w, "server no longer assigned", http.StatusForbidden)
		return
	}

	g.mu.Lock()
	count := 0

	for _, item := range g.events {
		if item.ServerID == id {
			count++
		}
	}

	status := struct {
		Pending   int    `json:"pending"`
		Limit     int    `json:"limit"`
		NodeLimit int    `json:"node_limit"`
		Dropped   int    `json:"dropped"`
		LastError string `json:"last_error"`
	}{count, gameQueuePerServer, gameQueueMaximum, g.dropped[id], g.failures[id]}
	g.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(status); err != nil {
		slog.Debug("Write game queue status", "error", err)
	}
}
