package runtime

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"campfire/internal/shared/model"
)

type fakeDocker struct {
	mu               sync.Mutex
	states           map[string]string
	calls            [][]string
	failInstaller    bool
	block            <-chan struct{}
	installerStarted chan<- struct{}
	input            string
	installerScript  string
	inputError       error
}

type fakeQuota struct{ err error }

func (q fakeQuota) Prepare(string, int64) error { return q.err }
func (q fakeQuota) Verify(string, int64) error  { return q.err }

func newTestManager(ctx context.Context, directory string) *Manager {
	m := New(ctx, directory)
	m.Quota = fakeQuota{}

	return m
}

func (f *fakeDocker) Run(ctx context.Context, input io.Reader, args ...string) (string, error) {
	if args[0] == "run" && f.block != nil {
		if f.installerStarted != nil {
			f.installerStarted <- struct{}{}
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-f.block:
		}
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls = append(f.calls, append([]string(nil), args...))

	name := args[len(args)-1]
	switch args[0] {
	case "run":
		if input != nil {
			data, err := io.ReadAll(input)
			if err != nil {
				return "", err
			}

			f.installerScript = string(data)
		}

		if f.failInstaller {
			return "password=private-value", errors.New("installer failed")
		}

		return "Installed", nil
	case "inspect":
		state, ok := f.states[name]
		if !ok {
			return "Error: No such object", errors.New("missing")
		}

		if len(args) > 2 && args[2] == "{{json .State}}" {
			return `{"Status":"` + state + `","StartedAt":"2026-01-01T00:00:00Z","FinishedAt":"0001-01-01T00:00:00Z"}`, nil
		}

		return state, nil
	case "create":
		for i, arg := range args {
			if arg == "--name" {
				f.states[args[i+1]] = "created"
			}
		}

		return "container-id", nil
	case "start":
		f.states[name] = "running"
	case "stop", "kill":
		f.states[name] = "exited"
	case "rm":
		delete(f.states, name)
	case "logs":
		return "Ready\n", nil
	}

	return "", nil
}

func (f *fakeDocker) Input(ctx context.Context, id, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.inputError != nil {
		return f.inputError
	}

	f.input = text
	if text == "stop" {
		f.states[id] = "exited"
	}

	return ctx.Err()
}

func runtimeFixture() model.Deployment {
	return model.Deployment{ServerID: "test-server", Configuration: model.ServerConfiguration{Name: "Game", SparkID: "node", EmberID: "ember", Image: "alpine:3.22", CPULimit: 0.5, MemoryMB: 128, DiskMB: 1024, Variables: map[string]string{"SECONDS": "3600"}, Ports: []model.ServerPort{{BindIP: "127.0.0.1", HostPort: 25565, ContainerPort: 25565, Protocol: "tcp"}}}, Ember: model.Ember{ID: "ember", Schema: 1, Name: "Game", Images: map[string]string{"Alpine": "alpine:3.22"}, Startup: "sleep {{SECONDS}}", WorkDir: "/home/container", User: "1000:1000", Ready: "Ready", Installer: &model.Installer{Image: "alpine:3.22", Entrypoint: "sh", Script: "echo install"}}}
}

func awaitOperation(t *testing.T, m *Manager, d model.Deployment, action, id string) model.RuntimeStatus {
	t.Helper()
	return awaitRequest(t, m, d, model.RuntimeRequest{ServerID: d.ServerID, Action: action, OperationID: id})
}

func awaitRequest(t *testing.T, m *Manager, d model.Deployment, request model.RuntimeRequest) model.RuntimeStatus {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	status, err := m.Execute(ctx, d, request)
	if err != nil {
		t.Fatal(err)
	}

	for status.Busy {
		select {
		case <-ctx.Done():
			t.Fatal("operation did not finish")
		case <-time.After(5 * time.Millisecond):
		}

		status, err = m.Execute(ctx, d, model.RuntimeRequest{ServerID: d.ServerID, Action: "status"})
		if err != nil {
			t.Fatal(err)
		}
	}

	return status
}

func TestRuntimeDisabledGameIntegrationHasNoMount(t *testing.T) {
	d := runtimeFixture()
	m := newTestManager(t.Context(), t.TempDir())
	t.Cleanup(m.Close)
	docker := &fakeDocker{states: make(map[string]string)}
	m.Docker = docker
	m.Ownership = func(string, string) error { return nil }
	m.GameIntegrationMount = func(string) (string, error) {
		t.Error("disabled server requested an integration endpoint")
		return t.TempDir(), nil
	}
	for _, action := range []string{"install", "start"} {
		if status := awaitOperation(t, m, d, action, action); status.Error != "" {
			t.Fatal(status.Error)
		}
	}
	docker.mu.Lock()
	defer docker.mu.Unlock()
	for _, args := range docker.calls {
		if args[0] == "create" && strings.Contains(strings.Join(args, " "), "campfire-game") {
			t.Fatal("disabled integration exposed a mount or socket environment", args)
		}
	}
}

func TestRuntimeLifecyclePersistenceAndDockerArguments(t *testing.T) {
	d := runtimeFixture()
	dir := t.TempDir()
	docker := &fakeDocker{states: make(map[string]string)}
	m := newTestManager(context.Background(), dir)
	m.Docker = docker
	m.Ownership = func(string, string) error { return nil }
	gameDirectory := t.TempDir()
	d.Configuration.GameIntegration = true
	m.GameIntegrationMount = func(id string) (string, error) {
		if id != d.ServerID {
			t.Error("integration mount requested for wrong server", id)
		}

		return gameDirectory, nil
	}
	t.Cleanup(m.Close)

	if _, err := m.Execute(context.Background(), d, model.RuntimeRequest{ServerID: d.ServerID, Action: "start", OperationID: "early"}); err == nil {
		t.Fatal("start before install succeeded")
	}

	if status := awaitOperation(t, m, d, "install", "install-1"); status.Error != "" || status.State != "stopped" {
		t.Fatalf("install: %+v", status)
	}

	if status := awaitOperation(t, m, d, "start", "start-1"); status.Error != "" || status.State != "running" || !status.Ready {
		t.Fatalf("start: %+v", status)
	}

	if _, err := m.Execute(context.Background(), d, model.RuntimeRequest{ServerID: d.ServerID, Action: "command", Content: "hello"}); err != nil {
		t.Fatal(err)
	}

	if _, err := m.Execute(context.Background(), d, model.RuntimeRequest{ServerID: d.ServerID, Action: "command", Content: "line1\nline2"}); err == nil {
		t.Fatal("multiline console input accepted")
	}

	docker.mu.Lock()
	calls := append([][]string(nil), docker.calls...)
	docker.mu.Unlock()

	var create []string

	for _, args := range calls {
		if args[0] == "create" {
			create = args
		}
	}

	joined := strings.Join(create, " ")
	for _, setting := range []string{"type=bind,source=" + gameDirectory + ",target=/run/campfire-game,readonly", "--env CAMPFIRE_GAME_SOCKET=/run/campfire-game/publish.sock"} {
		if !strings.Contains(joined, setting) {
			t.Error("missing isolated game integration mount", setting)
		}
	}

	for _, expected := range []string{"--user 1000:1000", "--cpus 0.5", "--memory 128m", "--memory-swap 128m", "--publish 127.0.0.1:25565:25565/tcp", "--cap-drop ALL", "--env TINI_SUBREAPER=1", "sh -c sleep ${SECONDS}"} {
		if !strings.Contains(joined, expected) {
			t.Errorf("missing container setting %s", expected)
		}
	}

	for index, action := range []string{"stop", "start", "restart", "kill"} {
		status := awaitOperation(t, m, d, action, action+strings.Repeat("x", index))
		if status.Error != "" {
			t.Fatalf("%s: %+v", action, status)
		}
	}

	m.Close()
	m = newTestManager(context.Background(), dir)
	m.Docker = docker
	m.Ownership = func(string, string) error { return nil }
	t.Cleanup(m.Close)

	if status := awaitOperation(t, m, d, "start", "after-restart"); status.State != "running" || status.Error != "" {
		t.Fatalf("runtime restart: %+v", status)
	}

	d.Ember.Stop = "stop"
	if status := awaitOperation(t, m, d, "stop", "console-stop"); status.Error != "" {
		t.Fatal(status.Error)
	}

	if docker.input != "stop" {
		t.Fatal("Ember stop command was not sent")
	}

	if status := awaitOperation(t, m, d, "delete", "delete-1"); status.State != "deleted" || status.Error != "" {
		t.Fatalf("delete: %+v", status)
	}

	if _, err := os.Stat(filepath.Join(dir, d.ServerID, "data")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("game data survived deletion")
	}
}

func TestInstallerNormalizesWindowsLineEndings(t *testing.T) {
	m := newTestManager(t.Context(), t.TempDir())
	t.Cleanup(m.Close)

	docker := &fakeDocker{states: make(map[string]string)}
	m.Docker = docker
	m.Ownership = func(string, string) error { return nil }
	d := runtimeFixture()

	d.Ember.Installer.Script = "#!/bin/bash\r\nif true; then\r\n  printf 'installed\\n'\r\nelif false; then\r\n  exit 1\r\nfi\r\n"
	if status := awaitOperation(t, m, d, "install", "crlf"); status.ErrorCode != "" || status.OperationState != model.OperationSucceeded {
		t.Fatal(status)
	}

	docker.mu.Lock()
	script := docker.installerScript
	docker.mu.Unlock()

	command := exec.CommandContext(t.Context(), "bash", "-s")
	command.Stdin = strings.NewReader(script)

	output, err := command.CombinedOutput()
	if err != nil || string(output) != "installed\n" || strings.Contains(script, "\r\n") {
		t.Fatal("Windows line endings reached the installer", string(output), err)
	}
}

type shellInstallerDocker struct{ fakeDocker }

func TestStopFallsBackWhenConsoleCannotAttach(t *testing.T) {
	m := newTestManager(t.Context(), t.TempDir())
	t.Cleanup(m.Close)

	d := runtimeFixture()
	d.Ember.Stop = "stop"
	m.Docker = &fakeDocker{states: map[string]string{container(d.ServerID): "running"}, inputError: errors.New("container restarted during attach")}

	e, err := m.load(d.ServerID)
	if err != nil {
		t.Fatal(err)
	}

	e.Installed = true

	if status := awaitOperation(t, m, d, "stop", "attach-failed"); status.State != "stopped" || status.ErrorCode != "" {
		t.Fatal("console failure prevented stopping", status)
	}
}

func (f *shellInstallerDocker) Run(ctx context.Context, input io.Reader, args ...string) (string, error) {
	if args[0] != "run" {
		return f.fakeDocker.Run(ctx, input, args...)
	}

	image := slices.Index(args, "alpine:3.22")
	if image < 0 {
		return "", errors.New("unexpected installer image")
	}

	command := exec.CommandContext(ctx, "bash", args[image+1:]...)
	command.Stdin = input
	output, err := command.CombinedOutput()

	return string(output), err
}

func TestInstallerStopsOnFailureBeforeSuccessMessage(t *testing.T) {
	m := newTestManager(t.Context(), t.TempDir())
	t.Cleanup(m.Close)
	m.Docker = &shellInstallerDocker{fakeDocker{states: make(map[string]string)}}
	m.Ownership = func(string, string) error { return nil }
	d := runtimeFixture()
	d.Ember.Installer.Script = "printf 'Download failed\\n'\nfalse\nprintf 'Install Complete\\n'\n"

	status := awaitOperation(t, m, d, "install", "script-fails")
	if status.OperationState != model.OperationFailed || status.ErrorCode != model.RuntimeInstallerFailed {
		t.Fatal("installer falsely reported success", status)
	}

	output, err := os.ReadFile(filepath.Join(m.root(d.ServerID), "installer.log"))
	if err != nil || string(output) != "Download failed\n" {
		t.Fatal("installer continued after failure", string(output), err)
	}

	d.Ember.Installer.Script = "printf 'Retry succeeded\\n'\n"

	status = awaitOperation(t, m, d, "install", "script-retry")
	if status.OperationState != model.OperationSucceeded || status.ErrorCode != "" {
		t.Fatal("installer retry did not succeed", status)
	}

	output, err = os.ReadFile(filepath.Join(m.root(d.ServerID), "installer.log"))
	if err != nil || string(output) != "Retry succeeded\n" {
		t.Fatal("installer retry retained old logs", string(output), err)
	}
}

func TestInterruptInstaller(t *testing.T) {
	for _, action := range []string{"stop", "kill"} {
		t.Run(action, func(t *testing.T) {
			d := runtimeFixture()
			blocked := make(chan struct{})
			started := make(chan struct{}, 1)
			docker := &fakeDocker{states: make(map[string]string), block: blocked, installerStarted: started}
			m := newTestManager(context.Background(), t.TempDir())
			m.Docker = docker
			m.Ownership = func(string, string) error { return nil }
			t.Cleanup(m.Close)

			request := model.RuntimeRequest{ServerID: d.ServerID, Action: "install", OperationID: "installation"}
			if _, err := m.Execute(context.Background(), d, request); err != nil {
				t.Fatal(err)
			}

			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("installer did not start")
			}

			if _, err := m.Execute(context.Background(), d, model.RuntimeRequest{ServerID: d.ServerID, Action: action, OperationID: "interrupt"}); err != nil {
				t.Fatal(err)
			}

			status := awaitRequest(t, m, d, request)
			if status.State != "not_installed" || status.OperationState != model.OperationInterrupted || status.ErrorCode != model.RuntimeOperationInterrupted {
				t.Fatalf("interrupted installer: %+v", status)
			}

			docker.mu.Lock()
			calls := append([][]string(nil), docker.calls...)
			docker.mu.Unlock()

			want := []string{"stop", "--time", "10", container(d.ServerID) + "-install"}
			if action == "kill" {
				want = []string{"stop", "--signal", "SIGKILL", "--time", "0", container(d.ServerID) + "-install"}
			}

			if !slices.ContainsFunc(calls, func(args []string) bool { return slices.Equal(args, want) }) {
				t.Fatalf("did not target the installer: %v", calls)
			}

			close(blocked)

			status = awaitOperation(t, m, d, "install", "retry")
			if status.OperationState != model.OperationSucceeded || status.State != "stopped" {
				t.Fatalf("retry after interruption: %+v", status)
			}
		})
	}
}

func TestFailedInstallBusyAndCancellation(t *testing.T) {
	d := runtimeFixture()
	block := make(chan struct{})
	docker := &fakeDocker{states: make(map[string]string), block: block, failInstaller: true}
	m := newTestManager(context.Background(), t.TempDir())
	m.Docker = docker
	m.Ownership = func(string, string) error { return nil }

	status, err := m.Execute(context.Background(), d, model.RuntimeRequest{ServerID: d.ServerID, Action: "install", OperationID: "install-1"})
	if err != nil || !status.Busy {
		t.Fatalf("queue: %v", err)
	}

	if _, err := m.Execute(context.Background(), d, model.RuntimeRequest{ServerID: d.ServerID, Action: "delete", OperationID: "delete"}); err == nil {
		t.Fatal("concurrent destructive operation accepted")
	}

	repeated, err := m.Execute(context.Background(), d, model.RuntimeRequest{ServerID: d.ServerID, Action: "install", OperationID: "install-1"})
	if err != nil || repeated.OperationID != status.OperationID {
		t.Fatal("operation replay was not idempotent")
	}

	close(block)

	status = awaitOperation(t, m, d, "install", "install-1")
	if status.Error == "" || status.State != "not_installed" || strings.Contains(status.Error, "private-value") {
		t.Fatalf("failed installer state: %+v", status)
	}

	if _, err := m.Execute(context.Background(), d, model.RuntimeRequest{ServerID: d.ServerID, Action: "start", OperationID: "start"}); err == nil {
		t.Fatal("failed installer allowed startup")
	}

	m.Close()

	blocked := make(chan struct{})
	docker = &fakeDocker{states: make(map[string]string), block: blocked}
	m = newTestManager(context.Background(), t.TempDir())

	m.Docker = docker
	if _, err := m.Execute(context.Background(), d, model.RuntimeRequest{ServerID: d.ServerID, Action: "install", OperationID: "cancel"}); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})

	go func() { m.Close(); close(done) }()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not cancel installation")
	}
}

func TestConfigurationContainmentAndDiskAllocation(t *testing.T) {
	d := runtimeFixture()
	directory := t.TempDir()

	outside := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(outside, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink(outside, filepath.Join(directory, "config.json")); err != nil {
		t.Fatal(err)
	}

	d.Ember.Configs = []model.ConfigFile{{Path: "config.json", Parser: "json", Values: map[string]string{"value": "1"}}}
	if err := configure(directory, d); err == nil {
		t.Fatal("outside symlink accepted")
	}

	if err := os.Remove(filepath.Join(directory, "config.json")); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(directory, "config.json"), []byte(`{"value":0}`), 0600); err != nil {
		t.Fatal(err)
	}

	if err := configure(directory, d); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(directory, "large"), make([]byte, (1<<20)+1), 0600); err != nil {
		t.Fatal(err)
	}

	if err := diskCheck(directory, 1); err == nil {
		t.Fatal("disk allocation exceeded without error")
	}
}
