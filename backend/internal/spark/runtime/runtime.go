package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"campfire/internal/shared/model"
)

var safeID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

var startupVariables = regexp.MustCompile(`\{\{([A-Z][A-Z0-9_]*)\}\}`)

// Manager shares lifecycle behavior between the panel and remote Spark processes.
type Manager struct {
	Directory     string
	Docker        Executor
	Ownership     func(string, string) error
	FileOwner     func(*os.File, string) error
	Quota         DiskQuota
	ctx           context.Context
	cancel        context.CancelFunc
	mu            sync.Mutex
	entries       map[string]*entry
	wg            sync.WaitGroup
	closed        bool
	metricsMu     sync.Mutex
	metrics       map[string]*metricsEntry
	transferTasks map[string]context.CancelFunc
}

type entry struct {
	mu        sync.Mutex
	uploads   map[string]*fileUpload
	Installed bool                `json:"installed"`
	Revision  string              `json:"revision"`
	Status    model.RuntimeStatus `json:"status"`
}

// New constructs a manager whose background operations follow the process lifetime.
func New(ctx context.Context, directory string) *Manager {
	ctx, cancel := context.WithCancel(ctx)
	return &Manager{Directory: directory, Docker: Docker{}, Ownership: ownership, FileOwner: assignFileOwner, Quota: &projectQuotas{registry: filepath.Join(directory, ".quota-projects"), kernel: linuxProjectKernel{}}, ctx: ctx, cancel: cancel, entries: make(map[string]*entry)}
}

// Close prevents new operations and waits for in-flight work after process cancellation.
func (m *Manager) Close() { m.mu.Lock(); m.closed = true; m.mu.Unlock(); m.cancel(); m.wg.Wait() }

func (m *Manager) root(id string) string { return filepath.Join(m.Directory, id) }
func container(id string) string         { return "campfire-" + id }

func (m *Manager) load(id string) (*entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, errors.New("runtime is shutting down")
	}

	if e := m.entries[id]; e != nil {
		return e, nil
	}

	e := &entry{Status: model.RuntimeStatus{ServerID: id, State: "not_installed"}}

	data, err := os.ReadFile(filepath.Join(m.root(id), "state.json"))
	if err == nil {
		if err := json.Unmarshal(data, e); err != nil {
			return nil, err
		}

		if e.Status.Busy {
			e.Status.Busy = false
			e.Status.Error = "operation interrupted by Spark restart"
			e.Status.Progress = "Interrupted"
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	m.entries[id] = e
	e.Status.DiskQuotaEnforced = false

	return e, nil
}

func (m *Manager) persist(id string, e *entry) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}

	return writeAtomic(filepath.Join(m.root(id), "state.json"), data, 0600)
}

// Execute starts a bounded asynchronous operation, or returns current Docker state.
func (m *Manager) Execute(ctx context.Context, d model.Deployment, q model.RuntimeRequest) (model.RuntimeStatus, error) {
	if err := ctx.Err(); err != nil {
		return model.RuntimeStatus{}, err
	}

	if !safeID.MatchString(q.ServerID) || q.ServerID != d.ServerID {
		return model.RuntimeStatus{}, errors.New("invalid server identity")
	}

	if q.Action == "metrics" {
		usage, err := m.Measure(ctx, d)
		return model.RuntimeStatus{ServerID: d.ServerID, State: usage.State, Metrics: &usage}, err
	}

	e, err := m.load(q.ServerID)
	if err != nil {
		return model.RuntimeStatus{}, err
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if transfer, err := m.localTransfer(q.ServerID); err != nil {
		return model.RuntimeStatus{}, err
	} else if transfer != nil && transfer.State != "active" && transfer.State != "released" {
		if q.Action != "status" && q.Action != "logs" {
			return model.RuntimeStatus{}, errors.New("server is locked for a Spark transfer")
		}

		if q.Action == "status" {
			status, err := m.observe(ctx, d, e)
			status.Busy = transfer.State != "retired" && transfer.State != "discarded"
			status.Progress, status.Operation, status.OperationID = "Spark transfer: "+transfer.State, "transfer", transfer.ID
			status.Error = transfer.Error

			return status, err
		}
	}

	if q.Action == "status" {
		if e.Status.Busy {
			return e.Status, nil
		}

		status, err := m.observe(ctx, d, e)
		if err != nil {
			return model.RuntimeStatus{}, err
		}

		e.Status.State, e.Status.Ready = status.State, status.Ready
		e.Status.DiskQuotaEnforced = status.DiskQuotaEnforced

		return e.Status, nil
	}

	if q.Action == "logs" {
		if !e.Installed {
			result := e.Status

			data, err := os.ReadFile(filepath.Join(m.root(d.ServerID), "installer.log"))
			if err == nil {
				result.Logs = string(data[:min(len(data), 65536)])
				return result, nil
			}

			output, err := m.Docker.Run(ctx, nil, "logs", "--tail", "200", container(d.ServerID)+"-install")
			if err != nil {
				output = "Installer output is not available yet."
			}

			result.Logs = output

			return result, nil
		}

		output, err := m.Docker.Run(ctx, nil, "logs", "--tail", "200", container(d.ServerID))
		if err != nil {
			return model.RuntimeStatus{}, err
		}

		result := e.Status
		result.Logs = output

		return result, nil
	}

	if q.Action == "command" {
		if q.Content == "" || len(q.Content) > 4096 || strings.ContainsAny(q.Content, "\r\n\x00") {
			return model.RuntimeStatus{}, errors.New("console command must be a single line of at most 4096 bytes")
		}

		state, err := m.inspect(ctx, d.ServerID)
		if err != nil {
			return model.RuntimeStatus{}, err
		}

		if state != "running" {
			return model.RuntimeStatus{}, errors.New("server is not running")
		}

		input, ok := m.Docker.(interface {
			Input(context.Context, string, string) error
		})
		if !ok {
			return model.RuntimeStatus{}, errors.New("console input is unavailable")
		}

		if err := input.Input(ctx, container(d.ServerID), q.Content); err != nil {
			return model.RuntimeStatus{}, err
		}

		return e.Status, nil
	}

	switch q.Action {
	case "install", "update", "reinstall", "start", "stop", "restart", "kill", "delete", "backup", "restore":
	default:
		return model.RuntimeStatus{}, errors.New("unsupported runtime operation")
	}

	if !safeID.MatchString(q.OperationID) {
		return model.RuntimeStatus{}, errors.New("operation ID is required")
	}

	if q.OperationID == e.Status.OperationID {
		return e.Status, nil
	}

	if e.Status.Busy {
		return model.RuntimeStatus{}, errors.New("another server operation is in progress")
	}

	if q.Action == "backup" || q.Action == "restore" || q.Action == "install" || q.Action == "update" || q.Action == "reinstall" {
		if q.Action == "restore" && !backupName(q.Content) {
			return model.RuntimeStatus{}, errors.New("select a valid backup")
		}

		if err := m.backupStopped(ctx, d.ServerID); err != nil {
			return model.RuntimeStatus{}, err
		}
	}

	if q.Action != "install" && q.Action != "update" && q.Action != "reinstall" && q.Action != "delete" && !e.Installed {
		return model.RuntimeStatus{}, errors.New("install the server first")
	}

	if q.Action == "install" && e.Installed {
		return model.RuntimeStatus{}, errors.New("server is already installed")
	}

	phase := map[string]string{"install": "installing", "update": "updating", "reinstall": "reinstalling", "start": "starting", "stop": "stopping", "restart": "restarting", "kill": "stopping", "delete": "deleting"}[q.Action]
	if q.Action == "backup" || q.Action == "restore" {
		phase = "stopped"
	}

	e.Status = model.RuntimeStatus{ServerID: q.ServerID, State: phase, OperationID: q.OperationID, Operation: q.Action, Progress: "Queued", Busy: true}
	if err := m.persist(q.ServerID, e); err != nil {
		e.Status.Busy = false
		return model.RuntimeStatus{}, err
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()

		e.Status.Busy = false

		return model.RuntimeStatus{}, errors.New("runtime is shutting down")
	}

	m.wg.Add(1)

	m.mu.Unlock()
	go m.run(d, q, e)

	return e.Status, nil
}

func (m *Manager) progress(_ string, e *entry, text string) {
	e.mu.Lock()
	e.Status.Progress = text
	e.mu.Unlock()
}

func (m *Manager) run(d model.Deployment, q model.RuntimeRequest, e *entry) {
	defer m.wg.Done()

	ctx, cancel := context.WithTimeout(m.ctx, 45*time.Minute)
	defer cancel()

	var err error

	switch q.Action {
	case "backup":
		err = m.createBackup(ctx, d, q.OperationID, e)
	case "restore":
		if q.BackupSource != nil {
			err = m.downloadBackup(ctx, d, q.Content, q.BackupSource, e)
		}

		if err == nil {
			err = m.restoreBackup(ctx, d, q.Content, e)
		}
	default:
		err = m.perform(ctx, d, q.Action, e)
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	e.Status.Busy = false

	e.Status.Progress = "Complete"
	if err != nil {
		e.Status.Error = err.Error()
		e.Status.Progress = "Failed"
	}

	status, observeErr := m.observe(ctx, d, e)
	if observeErr == nil {
		e.Status.State, e.Status.Ready = status.State, status.Ready
		e.Status.DiskQuotaEnforced = status.DiskQuotaEnforced
	} else {
		e.Status.State = "unknown"
		if e.Status.Error == "" {
			e.Status.Error = "could not inspect Docker state"
		}
	}

	if q.Action == "delete" && err == nil {
		e.Status.State = "deleted"
	}

	if err := m.persist(d.ServerID, e); err != nil {
		e.Status.Error = "could not persist runtime state"
		e.Status.Progress = "Failed"
	}
}

func (m *Manager) inspect(ctx context.Context, id string) (string, error) {
	out, err := m.Docker.Run(ctx, nil, "inspect", "--format", "{{.State.Status}}", container(id))
	if err != nil {
		lower := strings.ToLower(out)
		if strings.Contains(lower, "no such object") || strings.Contains(lower, "no such container") {
			return "missing", nil
		}

		return "", err
	}

	return strings.TrimSpace(out), nil
}

func (m *Manager) observe(ctx context.Context, d model.Deployment, e *entry) (model.RuntimeStatus, error) {
	state, err := m.inspect(ctx, d.ServerID)
	if err != nil {
		return model.RuntimeStatus{}, err
	}

	status := model.RuntimeStatus{ServerID: d.ServerID, State: state}
	if volume, err := m.volume(d.ServerID); err == nil {
		status.DiskQuotaEnforced = m.Quota.Verify(volume, d.Configuration.DiskMB) == nil
	}

	if state == "missing" {
		status.State = "not_installed"
		if e.Installed {
			status.State = "stopped"
		}

		if e.Status.State == "deleted" {
			status.State = "deleted"
		}
	}

	if state == "exited" || state == "created" {
		status.State = "stopped"
	}

	if state == "running" {
		status.Ready = d.Ember.Ready == ""
		if d.Ember.Ready != "" {
			logs, err := m.Docker.Run(ctx, nil, "logs", "--tail", "200", container(d.ServerID))
			status.Ready = err == nil && strings.Contains(logs, d.Ember.Ready)
		}
	}

	return status, nil
}

func (m *Manager) perform(ctx context.Context, d model.Deployment, action string, e *entry) error {
	switch action {
	case "install":
		return m.install(ctx, d, e)
	case "update", "reinstall":
		return m.reinstall(ctx, d, e, action == "update")
	case "start":
		return m.start(ctx, d, e)
	case "restart":
		if err := m.stop(ctx, d); err != nil {
			return err
		}

		return m.start(ctx, d, e)
	case "stop":
		return m.stop(ctx, d)
	case "kill":
		state, err := m.inspect(ctx, d.ServerID)
		if err != nil {
			return err
		}

		if state == "missing" || state == "exited" || state == "created" {
			return nil
		}

		_, err = m.Docker.Run(ctx, nil, "kill", container(d.ServerID))

		return err
	case "delete":
		m.progress(d.ServerID, e, "Removing containers")

		for _, name := range []string{container(d.ServerID) + "-install", container(d.ServerID)} {
			out, err := m.Docker.Run(ctx, nil, "rm", "--force", name)
			if err != nil && !strings.Contains(strings.ToLower(out), "no such container") && !strings.Contains(strings.ToLower(out), "no such object") {
				return err
			}
		}

		for _, name := range []string{"data", "backups", "restore-previous"} {
			if err := os.RemoveAll(filepath.Join(m.root(d.ServerID), name)); err != nil {
				return err
			}
		}

		e.mu.Lock()
		e.Installed = false
		e.Revision = ""
		e.mu.Unlock()

		return nil
	}

	return errors.New("unsupported runtime operation")
}

func environment(d model.Deployment) map[string]string {
	env := make(map[string]string, len(d.Configuration.Variables)+3)
	for key, value := range d.Configuration.Variables {
		env[key] = value
	}

	env["SERVER_PORT"] = strconv.Itoa(d.Configuration.Ports[0].ContainerPort)

	env["SERVER_IP"] = "0.0.0.0"
	env["SERVER_MEMORY"] = strconv.FormatInt(d.Configuration.MemoryMB, 10)

	return env
}

func render(value string, env map[string]string) string {
	for key, replacement := range env {
		value = strings.ReplaceAll(value, "{{"+key+"}}", replacement)
	}

	return value
}

func environmentArgs(env map[string]string) []string {
	args := make([]string, 0, 2*len(env))
	for key, value := range env {
		args = append(args, "--env", key+"="+value)
	}

	return args
}

func (m *Manager) volume(id string) (string, error) {
	return filepath.Abs(filepath.Join(m.root(id), "data"))
}

func (m *Manager) install(ctx context.Context, d model.Deployment, e *entry) error {
	volume, err := m.volume(d.ServerID)
	if err != nil {
		return err
	}

	if strings.ContainsAny(volume, ",\n") {
		return errors.New("runtime path cannot contain commas or newlines")
	}

	if err := os.MkdirAll(volume, 0700); err != nil {
		return err
	}

	if err := m.Quota.Prepare(volume, d.Configuration.DiskMB); err != nil {
		return err
	}

	seccomp, err := m.seccompPath()
	if err != nil {
		return err
	}
	// A failed installer keeps its files. Explicit retry reruns its script in that directory.
	if installer := d.Ember.Installer; installer != nil {
		output, cleanupErr := m.Docker.Run(ctx, nil, "rm", "--force", container(d.ServerID)+"-install")
		if cleanupErr != nil && !strings.Contains(strings.ToLower(output), "no such container") && !strings.Contains(strings.ToLower(output), "no such object") {
			return cleanupErr
		}

		m.progress(d.ServerID, e, "Running Ember installer")
		args := make([]string, 0, 40+2*len(d.Configuration.Variables))
		args = append(args, "run", "--name", container(d.ServerID)+"-install", "--label", "campfire.server="+d.ServerID, "--memory", strconv.FormatInt(d.Configuration.MemoryMB, 10)+"m", "--cpus", strconv.FormatFloat(d.Configuration.CPULimit, 'f', -1, 64), "--pids-limit", "128", "--security-opt", "no-new-privileges", "--cap-drop", "ALL", "--cap-add", "CHOWN", "--cap-add", "DAC_OVERRIDE", "--cap-add", "FOWNER", "--mount", "type=bind,source="+volume+",target=/mnt/server", "--workdir", "/mnt/server", "--entrypoint", installer.Entrypoint, "-i")
		args = append(args, environmentArgs(environment(d))...)
		args = append(args, "--security-opt", "seccomp="+seccomp)
		args = append(args, installer.Image, "-s")
		output, runErr := m.Docker.Run(ctx, strings.NewReader(installer.Script), args...)
		err = runErr
		logErr := writeAtomic(filepath.Join(m.root(d.ServerID), "installer.log"), []byte(output), 0600)
		cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
		_, cleanupErr = m.Docker.Run(cleanup, nil, "rm", "--force", container(d.ServerID)+"-install")

		stop()

		if err != nil {
			return err
		}

		if cleanupErr != nil {
			return cleanupErr
		}

		if logErr != nil {
			return errors.New("could not save installer output")
		}
	}

	m.progress(d.ServerID, e, "Applying configuration and file ownership")

	if err := configure(volume, d); err != nil {
		return err
	}

	if err := m.Ownership(volume, d.Ember.User); err != nil {
		return err
	}

	if err := diskCheck(volume, d.Configuration.DiskMB); err != nil {
		return err
	}

	if err := m.Quota.Prepare(volume, d.Configuration.DiskMB); err != nil {
		return err
	}

	e.mu.Lock()
	e.Installed = true
	e.mu.Unlock()

	return nil
}

func (m *Manager) start(ctx context.Context, d model.Deployment, e *entry) error {
	if _, err := os.Lstat(filepath.Join(m.root(d.ServerID), "restore-previous")); !errors.Is(err, os.ErrNotExist) {
		return errors.New("previous restore data requires operator recovery before starting")
	}

	state, err := m.inspect(ctx, d.ServerID)
	if err != nil {
		return err
	}

	if state == "running" {
		volume, err := m.volume(d.ServerID)
		if err != nil {
			return err
		}

		return m.Quota.Verify(volume, d.Configuration.DiskMB)
	}

	if state != "missing" {
		if _, err := m.Docker.Run(ctx, nil, "rm", container(d.ServerID)); err != nil {
			return err
		}
	}

	volume, err := m.volume(d.ServerID)
	if err != nil {
		return err
	}

	if err := m.Quota.Prepare(volume, d.Configuration.DiskMB); err != nil {
		return err
	}

	if err := diskCheck(volume, d.Configuration.DiskMB); err != nil {
		return err
	}

	if err := configure(volume, d); err != nil {
		return err
	}

	if err := m.Ownership(volume, d.Ember.User); err != nil {
		return err
	}

	env := environment(d)
	if err := m.Quota.Prepare(volume, d.Configuration.DiskMB); err != nil {
		return err
	}

	seccomp, err := m.seccompPath()
	if err != nil {
		return err
	}

	startup := startupVariables.ReplaceAllStringFunc(d.Ember.Startup, func(value string) string { return "${" + value[2:len(value)-2] + "}" })
	env["STARTUP"] = startup
	args := []string{"create", "--name", container(d.ServerID), "--label", "campfire.server=" + d.ServerID, "--interactive", "--init", "--user", d.Ember.User, "--memory", strconv.FormatInt(d.Configuration.MemoryMB, 10) + "m", "--memory-swap", strconv.FormatInt(d.Configuration.MemoryMB, 10) + "m", "--cpus", strconv.FormatFloat(d.Configuration.CPULimit, 'f', -1, 64), "--pids-limit", "512", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--restart", "unless-stopped", "--log-driver", "local", "--log-opt", "max-size=10m", "--log-opt", "max-file=3", "--mount", "type=bind,source=" + volume + ",target=" + d.Ember.WorkDir, "--workdir", d.Ember.WorkDir}

	args = append(args, environmentArgs(env)...)

	args = append(args, "--security-opt", "seccomp="+seccomp)
	for _, port := range d.Configuration.Ports {
		args = append(args, "--publish", fmt.Sprintf("%s:%d:%d/%s", formatIP(port.BindIP), port.HostPort, port.ContainerPort, port.Protocol))
	}

	args = append(args, d.Configuration.Image)
	if startup != "" {
		args = append(args, "sh", "-c", startup)
	}

	m.progress(d.ServerID, e, "Creating container")

	if _, err := m.Docker.Run(ctx, nil, args...); err != nil {
		return err
	}

	_, err = m.Docker.Run(ctx, nil, "start", container(d.ServerID))

	return err
}

func formatIP(ip string) string {
	if strings.Contains(ip, ":") {
		return "[" + ip + "]"
	}

	return ip
}

func (m *Manager) stop(ctx context.Context, d model.Deployment) error {
	state, err := m.inspect(ctx, d.ServerID)
	if err != nil {
		return err
	}

	if state == "missing" || state == "exited" || state == "created" {
		return nil
	}

	args := []string{"stop", "--time", "30"}

	if d.Ember.Stop != "" && !strings.HasPrefix(d.Ember.Stop, "^") {
		input, ok := m.Docker.(interface {
			Input(context.Context, string, string) error
		})
		if !ok {
			return errors.New("ember stop command requires console input")
		}

		if err := input.Input(ctx, container(d.ServerID), d.Ember.Stop); err != nil {
			return err
		}

		deadline := time.NewTimer(30 * time.Second)
		defer deadline.Stop()

		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()

		waiting := true
		for waiting {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-deadline.C:
				waiting = false
			case <-ticker.C:
				state, err := m.inspect(ctx, d.ServerID)
				if err != nil {
					return err
				}

				if state != "running" {
					return nil
				}
			}
		}
	}

	if d.Ember.Stop == "^C" || d.Ember.Stop == "^SIGINT" {
		args = append(args, "--signal", "SIGINT")
	}

	args = append(args, container(d.ServerID))
	_, err = m.Docker.Run(ctx, nil, args...)

	return err
}
