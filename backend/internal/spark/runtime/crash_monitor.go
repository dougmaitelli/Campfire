package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"campfire/internal/shared/model"
	"campfire/internal/shared/utils"
)

// An OOM event alone can describe a child process that the server survived.
// Attribute it conservatively only to an immediately following SIGKILL exit.
// Delayed or incomplete event evidence remains an unknown crash reason.
const crashOOMExitWindow = 5 * time.Second

type crashMonitor struct {
	mu      sync.Mutex // protects only the journal map and stream health
	manager *Manager
	states  map[string]*crashState
	done    chan struct{}
	failure string
}

// SupportsCrashMonitoring reports whether the Docker adapter supplies events.
func (m *Manager) SupportsCrashMonitoring() bool {
	_, ok := m.Docker.(crashEventProvider)
	return ok
}

// WatchCrashes starts one node-wide event observer, independent of browsers and
// panel connections. It never changes Docker's existing restart policy.
func (m *Manager) WatchCrashes(d model.Deployment) error {
	if !utils.IsValidResourceID(d.ServerID) {
		return errors.New("invalid crash monitoring identity")
	}

	provider, ok := m.Docker.(crashEventProvider)
	if !ok {
		return nil
	}

	m.crashMu.Lock()
	if m.ctx.Err() != nil {
		m.crashMu.Unlock()
		return m.ctx.Err()
	}

	if m.crashes == nil {
		m.crashes = &crashMonitor{manager: m, states: map[string]*crashState{}, done: make(chan struct{})}
		go m.crashes.run(provider)
	}

	observer := m.crashes
	m.crashMu.Unlock()

	if state, err := observer.state(d.ServerID); err == nil {
		state.mu.Lock()
		_ = observer.save(d.ServerID, state)
		state.mu.Unlock()
	}

	return nil
}

func (m *Manager) crashObserver() *crashMonitor {
	m.crashMu.Lock()
	defer m.crashMu.Unlock()

	return m.crashes
}

func (c *crashMonitor) run(provider crashEventProvider) {
	defer close(c.done)

	ctx := c.manager.ctx

	pruned := make(chan struct{})
	go func() {
		defer close(pruned)

		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.prune()
			}
		}
	}()

	defer func() { <-pruned }()

	for ctx.Err() == nil {
		_ = provider.followCrashEvents(ctx, c.observe)
		if ctx.Err() != nil {
			return
		}

		c.mu.Lock()
		// The CLI has no readiness notification on a quiet reconnect. Describe an
		// interruption, not a continuing outage that we cannot establish.
		c.failure = "Crash monitoring was interrupted; retained evidence may be incomplete."
		c.mu.Unlock()

		timer := time.NewTimer(3 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (c *crashMonitor) observe(event crashEvent) error {
	id := event.Actor.Attributes["campfire.server"]
	if !utils.IsValidResourceID(id) || event.Actor.Attributes["name"] != container(id) || len(event.Actor.ID) != 64 || event.TimeNano <= 0 {
		return nil
	}

	if _, err := hex.DecodeString(event.Actor.ID); err != nil {
		return nil
	}
	// Valid incoming traffic establishes stream recovery independently of whether
	// it is a duplicate, belongs to a retired server, or creates a crash record.
	c.mu.Lock()
	c.failure = ""
	c.mu.Unlock()
	// Labels alone do not authorize runtime directories. Installers have a
	// different name, and only saved, installed runtime identities are observed.
	info, err := os.Lstat(filepath.Join(c.manager.root(id), "state.json"))
	if err != nil || !info.Mode().IsRegular() {
		return nil
	}

	runtimeState, err := c.manager.load(id)
	if err != nil {
		return nil
	}

	state, err := c.state(id)
	if err != nil {
		return nil
	} // snapshot reports this server's journal error

	state.mu.Lock()
	defer state.mu.Unlock()
	// The journal lock fences deletion, including observers already waiting when
	// cleanup removes a journal. A retained pointer must never recreate it.
	runtimeState.mu.Lock()
	deleted := !runtimeState.Installed || runtimeState.Status.State == model.RuntimeStateDeleting || runtimeState.Status.State == model.RuntimeStateDeleted
	runtimeState.mu.Unlock()

	if state.retired || deleted {
		return nil
	}

	if event.TimeNano < state.LastEvent || event.TimeNano == state.LastEvent && event.Action == state.LastAction {
		return nil
	}

	switch event.Action {
	case "start":
		state.ContainerID = event.Actor.ID
		if state.IntentID == event.Actor.ID && event.TimeNano > state.IntentAt {
			state.IntentID = ""
		}
	case "oom":
		state.ContainerID = event.Actor.ID
	case "kill":
		switch event.Actor.Attributes["signal"] {
		case "2", "9", "15":
			state.IntentID, state.IntentAt, state.IntentUntil = event.Actor.ID, event.TimeNano, time.Unix(0, event.TimeNano).Add(2*time.Minute).Unix()
		}
	case "die":
		code, err := strconv.Atoi(event.Actor.Attributes["exitCode"])
		if err != nil || code < 0 || code > 255 {
			return errors.New("invalid Docker exit code")
		}

		exited := time.Unix(0, event.TimeNano)

		intentional := state.IntentID == event.Actor.ID && event.TimeNano >= state.IntentAt && exited.Unix() <= state.IntentUntil
		if !intentional && code != 0 && exited.After(time.Now().Add(-model.CrashRetention)) {
			reason := model.CrashReasonUnknown
			if code == 137 && state.ContainerID == event.Actor.ID && state.LastAction == "oom" && state.LastEvent > 0 && event.TimeNano-state.LastEvent <= int64(crashOOMExitWindow) {
				reason = model.CrashReasonOutOfMemory
			}

			record := c.capture(event, code, reason)
			if err := record.Validate(); err != nil {
				return err
			}

			if !slices.ContainsFunc(state.Records, func(existing model.CrashRecord) bool { return existing.ID == record.ID }) {
				state.Records = append(state.Records, record)
			}
		}

		if state.IntentID == event.Actor.ID {
			state.IntentID = ""
		}
	default:
		return nil
	}

	state.LastEvent, state.LastAction = event.TimeNano, event.Action
	// A storage failure preserves bounded live evidence and cannot disconnect the
	// node-wide stream. Subsequent saves or background cleanup retry this journal.
	_ = c.save(id, state)

	return nil
}

func (c *crashMonitor) capture(event crashEvent, code int, reason model.CrashReason) model.CrashRecord {
	hash := sha256.Sum256([]byte(event.Actor.ID + ":" + strconv.FormatInt(event.TimeNano, 10)))
	exited := time.Unix(0, event.TimeNano)
	record := model.CrashRecord{ID: hex.EncodeToString(hash[:]), ContainerID: event.Actor.ID, ExitedAt: exited.Unix(), ExitCode: code, Reason: reason}

	ctx, cancel := context.WithTimeout(c.manager.ctx, 3*time.Second)
	defer cancel()
	// The inspect OOM flag can remain true after a nonfatal child OOM, so it is
	// not a crash-cause signal. Inspect only scopes console capture transiently.
	var startedAt time.Time

	text, err := c.manager.Docker.Run(ctx, nil, "inspect", "--format", "{{json .State}}", event.Actor.ID)
	if err == nil {
		var inspected struct{ StartedAt, FinishedAt time.Time }
		if json.Unmarshal([]byte(text), &inspected) == nil && !inspected.FinishedAt.IsZero() && !inspected.FinishedAt.After(exited) && inspected.FinishedAt.Unix() == record.ExitedAt && !inspected.StartedAt.After(exited) {
			startedAt = inspected.StartedAt
		}
	}

	args := []string{"logs", "--timestamps", "--tail", "200", "--until", exited.UTC().Format(time.RFC3339Nano)}
	if !startedAt.IsZero() {
		args = append(args, "--since", startedAt.UTC().Format(time.RFC3339Nano))
	}

	args = append(args, event.Actor.ID)

	output, err := c.manager.Docker.Run(ctx, nil, args...)
	if err != nil {
		record.ConsoleUnavailable = true
		return record
	}

	output = strings.ToValidUTF8(output, "�")

	record.ConsoleTruncated = len(output) > model.CrashConsoleBytes || strings.Count(output, "\n") >= 200
	if len(output) > model.CrashConsoleBytes {
		output = output[len(output)-model.CrashConsoleBytes:]
	}

	for !utf8.ValidString(output) {
		output = output[1:]
	}

	record.Console = output

	return record
}

func (m *Manager) crashSnapshot(id string) ([]model.CrashRecord, string) {
	observer := m.crashObserver()
	if observer == nil {
		return nil, ""
	}

	observer.mu.Lock()
	streamFailure := observer.failure
	observer.mu.Unlock()

	state, err := observer.state(id)
	if err != nil {
		return nil, "Saved crash evidence is unavailable."
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	if state.retired {
		return nil, streamFailure
	}

	records := slices.Clone(state.Records)
	cutoff := time.Now().Add(-model.CrashRetention).Unix()

	records = slices.DeleteFunc(records, func(record model.CrashRecord) bool { return record.ExitedAt < cutoff })
	if state.failure != "" {
		return records, state.failure
	}

	return records, streamFailure
}

// expectCrashStop records bounded intent before managed stops. Failures do not
// prevent power operations; intent is cleared if the same run remains alive.
func (m *Manager) expectCrashStop(ctx context.Context, id string) func() {
	observer := m.crashObserver()
	if observer == nil {
		return func() {}
	}

	text, err := m.Docker.Run(ctx, nil, "inspect", "--format", `{"id":{{json .Id}},"running":{{json .State.Running}}}`, container(id))
	if err != nil {
		return func() {}
	}

	var inspected struct {
		ID      string
		Running bool
	}
	if json.Unmarshal([]byte(text), &inspected) != nil || !inspected.Running {
		return func() {}
	}

	state, err := observer.state(id)
	if err != nil {
		return func() {}
	}

	state.mu.Lock()
	if state.retired {
		state.mu.Unlock()
		return func() {}
	}

	intentAt := time.Now().UnixNano()
	state.IntentID, state.IntentAt, state.IntentUntil = inspected.ID, intentAt, time.Now().Add(2*time.Minute).Unix()
	_ = observer.save(id, state)
	state.mu.Unlock()

	return func() {
		check, cancel := context.WithTimeout(m.ctx, 3*time.Second)
		defer cancel()

		text, err := m.Docker.Run(check, nil, "inspect", "--format", "{{.State.Running}}", inspected.ID)
		if err != nil || strings.TrimSpace(text) != "true" {
			return
		}

		state.mu.Lock()
		defer state.mu.Unlock()

		if !state.retired && state.IntentID == inspected.ID && state.IntentAt == intentAt {
			state.IntentID = ""
			_ = observer.save(id, state)
		}
	}
}
