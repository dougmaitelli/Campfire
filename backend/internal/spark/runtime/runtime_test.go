package runtime

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"campfire/internal/shared/model"
)

type fakeDocker struct {
	mu            sync.Mutex
	states        map[string]string
	calls         [][]string
	failInstaller bool
	block         <-chan struct{}
	input         string
}

func (f *fakeDocker) Run(ctx context.Context, _ io.Reader, args ...string) (string, error) {
	if args[0] == "run" && f.block != nil {
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
		if f.failInstaller {
			return "password=private-value", errors.New("installer failed")
		}

		return "Installed", nil
	case "inspect":
		state, ok := f.states[name]
		if !ok {
			return "Error: No such object", errors.New("missing")
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

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	status, err := m.Execute(ctx, d, model.RuntimeRequest{ServerID: d.ServerID, Action: action, OperationID: id})
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

func TestRuntimeLifecyclePersistenceAndDockerArguments(t *testing.T) {
	d := runtimeFixture()
	dir := t.TempDir()
	docker := &fakeDocker{states: make(map[string]string)}
	m := New(context.Background(), dir)
	m.Docker = docker
	m.Ownership = func(string, string) error { return nil }
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
	for _, expected := range []string{"--user 1000:1000", "--cpus 0.5", "--memory 128m", "--memory-swap 128m", "--publish 127.0.0.1:25565:25565/tcp", "--cap-drop ALL", "sleep ${SECONDS}"} {
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
	m = New(context.Background(), dir)
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

func TestFailedInstallBusyAndCancellation(t *testing.T) {
	d := runtimeFixture()
	block := make(chan struct{})
	docker := &fakeDocker{states: make(map[string]string), block: block, failInstaller: true}
	m := New(context.Background(), t.TempDir())
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
	m = New(context.Background(), t.TempDir())

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
