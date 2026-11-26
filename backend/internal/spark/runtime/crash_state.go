package runtime

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"campfire/internal/shared/model"
)

// Each journal has its own lock. The observer's map lock never spans filesystem
// or Docker I/O, so one server cannot hold up another server's status or intent.
type crashState struct {
	mu          sync.Mutex
	loaded      bool
	retired     bool
	failure     string
	LastEvent   int64               `json:"last_event"`
	LastAction  string              `json:"last_action"`
	ContainerID string              `json:"container_id"`
	IntentID    string              `json:"intent_id"`
	IntentAt    int64               `json:"intent_at"`
	IntentUntil int64               `json:"intent_until"`
	Records     []model.CrashRecord `json:"records"`
}

func (c *crashMonitor) filename(id string) string {
	return filepath.Join(c.manager.root(id), "crash-state.json")
}

func (c *crashMonitor) journal(id string) *crashState {
	c.mu.Lock()
	defer c.mu.Unlock()

	state := c.states[id]
	if state == nil {
		state = &crashState{}
		c.states[id] = state
	}

	return state
}

// state loads a bounded private journal. Callers lock the returned state before
// using it; retirement is checked again under that lock to fence deletion.
func (c *crashMonitor) state(id string) (*crashState, error) {
	state := c.journal(id)
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.loaded || state.retired {
		return state, nil
	}

	file, err := os.Open(c.filename(id))
	if err == nil {
		defer func() { _ = file.Close() }()

		data, readErr := io.ReadAll(io.LimitReader(file, 1<<20+1))
		// Decode into a separate value so a failed decode cannot partially replace
		// an existing journal or its synchronization state.
		var saved crashState
		if readErr != nil || len(data) > 1<<20 || json.Unmarshal(data, &saved) != nil || len(saved.Records) > model.CrashReplayLimit {
			return nil, errors.New("invalid saved crash state")
		}

		for _, record := range saved.Records {
			if err := record.Validate(); err != nil {
				return nil, err
			}
		}

		state.LastEvent, state.LastAction, state.ContainerID = saved.LastEvent, saved.LastAction, saved.ContainerID
		state.IntentID, state.IntentAt, state.IntentUntil = saved.IntentID, saved.IntentAt, saved.IntentUntil
		state.Records = saved.Records
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	state.loaded = true

	return state, nil
}

// save requires the journal lock, never the observer's map lock.
func (c *crashMonitor) save(id string, state *crashState) error {
	if state.retired {
		return nil
	}

	cutoff := time.Now().Add(-model.CrashRetention).Unix()

	state.Records = slices.DeleteFunc(state.Records, func(record model.CrashRecord) bool { return record.ExitedAt < cutoff })
	if len(state.Records) > model.CrashReplayLimit {
		state.Records = slices.Clone(state.Records[len(state.Records)-model.CrashReplayLimit:])
	}

	data, err := json.Marshal(state)
	if err == nil {
		err = writeAtomic(c.filename(id), data, 0600)
	}

	if err != nil {
		state.failure = "Crash evidence could not be saved; it may be lost if Spark restarts."
	} else {
		state.failure = ""
	}

	return err
}

func (c *crashMonitor) prune() {
	c.mu.Lock()

	states := make(map[string]*crashState, len(c.states))
	for id, state := range c.states {
		states[id] = state
	}
	c.mu.Unlock()

	cutoff := time.Now().Add(-model.CrashRetention).Unix()

	for id, state := range states {
		state.mu.Lock()

		expired := slices.ContainsFunc(state.Records, func(record model.CrashRecord) bool { return record.ExitedAt < cutoff })
		if expired || state.failure != "" {
			_ = c.save(id, state)
		}
		state.mu.Unlock()
	}
}

func (m *Manager) forgetCrashes(id string) error {
	observer := m.crashObserver()

	var state *crashState
	if observer != nil {
		state = observer.journal(id)
		state.mu.Lock()
		defer state.mu.Unlock()
	}

	err := os.Remove(filepath.Join(m.root(id), "crash-state.json"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if state != nil {
		state.retired = true
		state.Records = nil
		state.failure = ""

		observer.mu.Lock()
		delete(observer.states, id)
		observer.mu.Unlock()
	}

	return nil
}
