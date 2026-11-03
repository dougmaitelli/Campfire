package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"campfire/internal/shared/model"
	"campfire/internal/shared/utils"
)

const logMonitorInterval = 3 * time.Second
const logReconcileInterval = 30 * time.Second
const logLineLimit = 64 << 10
const logHistoryLimit = model.LogAlertHistoryLimit

type logContainer struct {
	ID        string    `json:"id"`
	StartedAt time.Time `json:"started_at"`
	Running   bool      `json:"running"`
	Status    string    `json:"status"`
}

type logProvider interface {
	inspectLogContainer(context.Context, string) (logContainer, error)
	followLogOutput(context.Context, string, string, func(string) error) error
}

func (d Docker) inspectLogContainer(ctx context.Context, name string) (logContainer, error) {
	text, err := d.Run(ctx, nil, "inspect", "--format", `{"id":{{json .Id}},"started_at":{{json .State.StartedAt}},"running":{{json .State.Running}},"status":{{json .State.Status}}}`, name)
	if err != nil {
		lower := strings.ToLower(text)
		if strings.Contains(lower, "no such object") || strings.Contains(lower, "no such container") {
			return logContainer{Status: "missing"}, nil
		}

		return logContainer{}, err
	}

	var result logContainer
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return result, errors.New("could not inspect log source")
	}

	return result, nil
}

func (Docker) followLogOutput(ctx context.Context, name, since string, emit func(string) error) error {
	command, err := dockerCommand(ctx, "logs", "--timestamps", "--follow", "--since", since, name)
	if err != nil {
		return err
	}

	return followCommand(ctx, command, emit)
}

type logSourceState struct {
	ContainerID string    `json:"container_id"`
	StartedAt   time.Time `json:"started_at"`
	ExecutionID string    `json:"execution_id"`
	Cursor      time.Time `json:"cursor"`
	Ready       bool      `json:"ready"`
}

type logMonitorState struct {
	ReadinessKey string                             `json:"readiness_key"`
	Conditions   string                             `json:"conditions"`
	Sources      map[model.LogPhase]*logSourceState `json:"sources"`
	Alerts       []model.DetectedLogAlert           `json:"alerts"`
}

type compiledLogAlert struct {
	definition model.LogAlert
	key        string
	matcher    model.LogMatcher
}

type logMonitor struct {
	mu           sync.Mutex
	ctx          context.Context
	cancel       context.CancelFunc
	done         chan struct{}
	provider     logProvider
	filename     string
	serverID     string
	definition   string
	readiness    model.LogMatcher
	rules        []compiledLogAlert
	state        logMonitorState
	failure      string
	sourceErrors map[model.LogPhase]string
	wake         chan struct{}
	lastSave     time.Time
}

func logHash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func (c logContainer) executionID() string {
	if c.ID == "" || c.StartedAt.IsZero() {
		return ""
	}

	return logHash(c.ID + "\x00" + c.StartedAt.Format(time.RFC3339Nano))
}

// SupportsLogMonitoring distinguishes the real streaming runtime from adapters.
func (m *Manager) SupportsLogMonitoring() bool {
	_, ok := m.Docker.(logProvider)
	return ok
}

// WatchLogs starts one shared readiness/error monitor per saved deployment.
// It follows the Spark process lifetime, independently of panel connections.
func (m *Manager) WatchLogs(d model.Deployment) error {
	if !utils.IsValidResourceID(d.ServerID) {
		return errors.New("invalid monitoring server identity")
	}

	entry, err := m.load(d.ServerID)
	if err != nil {
		return err
	}

	entry.mu.Lock()
	deleted := entry.Status.State == model.RuntimeStateDeleted
	entry.mu.Unlock()

	if deleted {
		return nil
	}

	provider, ok := m.Docker.(logProvider)
	if !ok {
		if len(d.Ember.LogAlerts) > 0 {
			return errors.New("runtime does not support log monitoring")
		}

		return nil
	}

	data, err := json.Marshal(d.Ember)
	if err != nil {
		return err
	}

	m.logMu.Lock()
	defer m.logMu.Unlock()

	if m.ctx.Err() != nil {
		return errors.New("runtime is shutting down")
	}

	if m.logMonitors == nil {
		m.logMonitors = make(map[string]*logMonitor)
	}

	if previous := m.logMonitors[d.ServerID]; previous != nil {
		if previous.definition == string(data) {
			return nil
		}

		previous.cancel()
		<-previous.done
		delete(m.logMonitors, d.ServerID)
	}

	if d.Ember.Ready == "" && d.Ember.Readiness == nil && len(d.Ember.LogAlerts) == 0 {
		return nil
	}

	ctx, cancel := context.WithCancel(m.ctx)
	monitor := &logMonitor{ctx: ctx, cancel: cancel, done: make(chan struct{}), provider: provider, filename: filepath.Join(m.root(d.ServerID), "log-state.json"), serverID: d.ServerID, definition: string(data), readiness: d.Ember.ReadinessMatcher()}
	monitor.state.Sources = make(map[model.LogPhase]*logSourceState)
	monitor.sourceErrors = make(map[model.LogPhase]string)
	monitor.wake = make(chan struct{}, 1)

	monitor.restoreState()

	keys := []string{}

	for _, rule := range d.Ember.LogAlerts {
		key := rule.Key()
		keys = append(keys, key)
		monitor.rules = append(monitor.rules, compiledLogAlert{definition: rule, key: key, matcher: model.CompileLogMatcher(rule.Any)})
	}

	slices.Sort(keys)

	readyRule := model.LogAlert{Any: []model.LogMatch{{Value: d.Ember.Ready}}}
	if d.Ember.Readiness != nil {
		readyRule.Any = d.Ember.Readiness.Any
	}

	readyKey := readyRule.Key()

	conditions, _ := json.Marshal([]any{readyKey, keys})
	if monitor.state.Conditions != string(conditions) {
		// New conditions must replay retained output, while the occurrence key
		// keeps previously detected and dismissed rules from firing again.
		for _, source := range monitor.state.Sources {
			if source != nil {
				source.Cursor = source.StartedAt
				if monitor.state.ReadinessKey != readyKey {
					source.Ready = false
				}
			}
		}

		monitor.state.Conditions = string(conditions)
	}

	monitor.state.ReadinessKey = readyKey

	m.logMonitors[d.ServerID] = monitor
	go monitor.run()

	return nil
}

func (m *logMonitor) restoreState() {
	file, err := os.Open(m.filename)
	if errors.Is(err, os.ErrNotExist) {
		return
	}

	if err != nil {
		m.failure = "Saved console monitoring state could not be read. Monitoring resumed from retained Docker logs."
		return
	}

	defer func() { _ = file.Close() }()

	stored, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil {
		m.failure = "Saved console monitoring state could not be read. Monitoring resumed from retained Docker logs."
		return
	}

	var state logMonitorState

	valid := len(stored) <= 1<<20 && json.Unmarshal(stored, &state) == nil && state.Sources != nil && len(state.Sources) <= 2 && len(state.Alerts) <= logHistoryLimit

	observation := model.LogObservation{Executions: make(map[model.LogPhase]string), Alerts: state.Alerts}
	if valid {
		for phase, source := range state.Sources {
			if source == nil || (logContainer{ID: source.ContainerID, StartedAt: source.StartedAt}).executionID() != source.ExecutionID || source.Cursor.Before(source.StartedAt) {
				valid = false
				break
			}

			observation.Executions[phase] = source.ExecutionID
		}
	}

	if valid && observation.Validate() == nil {
		m.state = state
		return
	}

	quarantine := m.filename + ".corrupt-" + time.Now().UTC().Format("20060102T150405.000000000")
	if err := os.Rename(m.filename, quarantine); err != nil {
		m.failure = "Invalid console monitoring state could not be quarantined. Monitoring resumed from retained Docker logs."
		return
	}

	m.failure = "Invalid console monitoring state was quarantined. Monitoring resumed from retained Docker logs."
}

func (m *Manager) stopLogMonitor(id string) {
	m.logMu.Lock()
	defer m.logMu.Unlock()

	if monitor := m.logMonitors[id]; monitor != nil {
		monitor.cancel()
		<-monitor.done
		delete(m.logMonitors, id)
	}
}

func (m *Manager) logSnapshot(id string) (*model.LogObservation, bool, bool) {
	m.logMu.Lock()
	monitor := m.logMonitors[id]
	m.logMu.Unlock()

	if monitor == nil {
		return nil, false, false
	}

	monitor.mu.Lock()
	defer monitor.mu.Unlock()

	observation := &model.LogObservation{Executions: make(map[model.LogPhase]string), Alerts: append([]model.DetectedLogAlert{}, monitor.state.Alerts...), Error: monitor.failure}
	if observation.Error == "" {
		for _, phase := range []model.LogPhase{model.LogRuntime, model.LogInstallation} {
			if monitor.sourceErrors[phase] != "" {
				observation.Error = monitor.sourceErrors[phase]
				break
			}
		}
	}

	for phase, source := range monitor.state.Sources {
		if source != nil {
			observation.Executions[phase] = source.ExecutionID
		}
	}

	source := monitor.state.Sources[model.LogRuntime]

	return observation, source != nil && source.Ready, true
}

func (m *Manager) wakeLogMonitor(id string) {
	m.logMu.Lock()
	defer m.logMu.Unlock()

	if monitor := m.logMonitors[id]; monitor != nil {
		select {
		case monitor.wake <- struct{}{}:
		default:
		}
	}
}

// Drain before removing the installer so even sub-second installations use the
// same matching and deduplication path as the live stream.
func (m *Manager) drainInstallerLogs(id string) {
	m.logMu.Lock()
	monitor := m.logMonitors[id]
	m.logMu.Unlock()

	if monitor == nil {
		return
	}

	needed := false
	for _, rule := range monitor.rules {
		needed = needed || rule.definition.Phase == "" || rule.definition.Phase == model.LogInstallation
	}

	if !needed {
		return
	}

	ctx, cancel := context.WithTimeout(monitor.ctx, 15*time.Second)
	defer cancel()

	name := container(id) + "-install"

	source, err := monitor.provider.inspectLogContainer(ctx, name)
	if err == nil && source.executionID() != "" {
		monitor.mu.Lock()
		state := monitor.sourceLocked(model.LogInstallation, source)
		since := state.Cursor.Format(time.RFC3339Nano)
		monitor.mu.Unlock()
		err = monitor.followSource(ctx, name, since, model.LogInstallation, source.executionID())
	}

	if err != nil {
		monitor.mu.Lock()
		monitor.failure = "Installer console monitoring could not finish before cleanup."
		monitor.mu.Unlock()
	}
}

func (m *logMonitor) sourceLocked(phase model.LogPhase, source logContainer) *logSourceState {
	state := m.state.Sources[phase]
	if state == nil || state.ExecutionID != source.executionID() {
		state = &logSourceState{ContainerID: source.ID, StartedAt: source.StartedAt, ExecutionID: source.executionID(), Cursor: source.StartedAt}
		m.state.Sources[phase] = state
		m.saveLocked()
	}

	return state
}

func (m *logMonitor) run() {
	defer close(m.done)

	var workers sync.WaitGroup

	for _, phase := range []model.LogPhase{model.LogRuntime, model.LogInstallation} {
		needed := phase == model.LogRuntime
		for _, rule := range m.rules {
			needed = needed || rule.definition.Phase == "" || rule.definition.Phase == phase
		}

		if !needed {
			continue
		}

		workers.Add(1)
		go func() { defer workers.Done(); m.runSource(phase) }()
	}

	workers.Wait()
	m.mu.Lock()
	defer m.mu.Unlock()

	m.saveLocked()
}

func (m *logMonitor) runSource(phase model.LogPhase) {
	name := container(m.serverID)

	var wake <-chan struct{}
	if phase == model.LogRuntime {
		wake = m.wake
	}

	if phase == model.LogInstallation {
		name += "-install"
	}

	var cancel context.CancelFunc

	var streamDone chan error

	var execution, completed string

	stop := func() {
		if cancel != nil {
			cancel()
			<-streamDone
			cancel, streamDone = nil, nil
		}
	}
	defer stop()

	for {
		ctx, finish := context.WithTimeout(m.ctx, 3*time.Second)
		source, err := m.provider.inspectLogContainer(ctx, name)

		finish()
		m.mu.Lock()
		if err != nil {
			m.sourceErrors[phase] = "Console monitoring could not inspect its Docker log source; retrying."
		} else {
			delete(m.sourceErrors, phase)
		}
		m.mu.Unlock()

		if err == nil && source.ID != "" && !source.StartedAt.IsZero() {
			key := source.executionID()
			if key != execution {
				stop()

				execution, completed = key, ""
			}

			if cancel == nil && (source.Running || completed != execution) {
				m.mu.Lock()

				state := m.sourceLocked(phase, source)

				since := state.Cursor.Format(time.RFC3339Nano)
				m.mu.Unlock()

				cancel, streamDone = m.startSource(name, since, phase, execution)
			}
		}

		delay := logMonitorInterval
		if err == nil && source.ID != "" && (cancel != nil || completed == execution) {
			delay = logReconcileInterval
		}

		timer := time.NewTimer(delay)
		select {
		case <-m.ctx.Done():
			timer.Stop()
			return
		case err := <-streamDone:
			timer.Stop()
			cancel()
			cancel, streamDone = nil, nil

			if err == nil {
				completed = execution
			} else if m.ctx.Err() == nil {
				m.mu.Lock()
				m.sourceErrors[phase] = "Console monitoring was interrupted; retrying from retained Docker logs."
				m.mu.Unlock()
			}

			select {
			case <-m.ctx.Done():
				return
			case <-time.After(time.Second):
			}
		case <-timer.C:
		case <-wake:
			timer.Stop()
		}
	}
}

func (m *logMonitor) startSource(name, since string, phase model.LogPhase, execution string) (context.CancelFunc, chan error) {
	ctx, cancel := context.WithCancel(m.ctx)
	done := make(chan error, 1)

	go func() {
		defer cancel()

		done <- m.followSource(ctx, name, since, phase, execution)
	}()

	return cancel, done
}

func (m *logMonitor) followSource(ctx context.Context, name, since string, phase model.LogPhase, execution string) error {
	pending := ""
	discarding := false
	emit := func(text string) error {
		for len(text) > 0 {
			line, rest, found := strings.Cut(text, "\n")
			if len(pending)+len(line) > logLineLimit {
				discarding, pending = true, ""

				m.mu.Lock()
				m.failure = "A console line exceeded the 64 KiB matching limit and was skipped."
				m.mu.Unlock()
			}

			if !discarding {
				pending += line
			}

			if !found {
				break
			}

			if !discarding {
				m.consumeLine(phase, execution, pending)
			}

			pending, discarding, text = "", false, rest
		}

		return nil
	}

	err := m.provider.followLogOutput(ctx, name, since, emit)
	if err == nil && pending != "" && !discarding {
		m.consumeLine(phase, execution, pending)
	}

	return err
}

func (m *logMonitor) consumeLine(phase model.LogPhase, execution, text string) {
	stamp, line, ok := strings.Cut(text, " ")
	if !ok {
		return
	}

	at, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	source := m.state.Sources[phase]
	if source == nil || source.ExecutionID != execution || at.Before(source.StartedAt) {
		return
	}

	line = strings.TrimSuffix(line, "\r")

	changed := false
	if phase == model.LogRuntime && !source.Ready && m.readiness.Matches(line) {
		source.Ready, changed = true, true
	}

	for _, rule := range m.rules {
		if rule.definition.Phase != "" && rule.definition.Phase != phase {
			continue
		}

		id := logHash(m.serverID + "\x00" + execution + "\x00" + rule.key)

		seen := false
		for _, alert := range m.state.Alerts {
			seen = seen || alert.ID == id
		}

		if seen || !rule.matcher.Matches(line) {
			continue
		}

		evidence := model.LogEvidence(line)

		m.state.Alerts = append(m.state.Alerts, model.DetectedLogAlert{ID: id, RuleKey: rule.key, ExecutionID: execution, Phase: phase, Title: rule.definition.Title, Message: rule.definition.Message, Severity: rule.definition.Severity, Action: rule.definition.Action, DetectedAt: at.Unix(), Line: evidence})
		m.state.Alerts = model.RetainLogAlerts(m.state.Alerts)

		changed = true
	}

	if at.After(source.Cursor) {
		source.Cursor = at
	}

	if changed || time.Since(m.lastSave) >= time.Second {
		m.saveLocked()
	}
}

func (m *logMonitor) saveLocked() {
	data, err := json.Marshal(m.state)
	if err == nil {
		err = writeAtomic(m.filename, data, 0600)
	}

	if err != nil {
		m.failure = "Log monitoring state could not be saved. Check the Spark's storage."
	}

	m.lastSave = time.Now()
}
