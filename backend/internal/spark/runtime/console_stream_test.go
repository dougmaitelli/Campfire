package runtime

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"campfire/internal/shared/model"
)

type startingConsoleDocker struct {
	mu       sync.Mutex
	created  bool
	checked  chan struct{}
	followed chan string
}

func (d *startingConsoleDocker) Run(context.Context, io.Reader, ...string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	select {
	case d.checked <- struct{}{}:
	default:
	}

	if !d.created {
		return "", nil
	}

	return "container-id", nil
}

func (d *startingConsoleDocker) Follow(_ context.Context, name string, emit func(string) error) error {
	d.followed <- name
	return emit("Installer output\n")
}

func TestConsoleWaitsForInstallerContainer(t *testing.T) {
	m := newTestManager(t.Context(), t.TempDir())
	t.Cleanup(m.Close)

	docker := &startingConsoleDocker{checked: make(chan struct{}, 1), followed: make(chan string, 1)}
	m.Docker = docker
	d := runtimeFixture()

	state, err := m.load(d.ServerID)
	if err != nil {
		t.Fatal(err)
	}

	state.Status.Busy, state.Status.Operation = true, "install"

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()

	finished := make(chan error, 1)
	go func() { finished <- m.StreamConsole(ctx, d, func(string) error { return nil }) }()

	select {
	case <-docker.checked:
	case <-ctx.Done():
		t.Fatal("console did not check container")
	}

	select {
	case <-docker.followed:
		t.Fatal("console followed a container before creation")
	default:
	}

	docker.mu.Lock()
	docker.created = true
	docker.mu.Unlock()

	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("console did not follow the created installer")
	}

	if name := <-docker.followed; name != container(d.ServerID)+"-install" {
		t.Fatal("wrong console target", name)
	}
}

func TestConsoleShowsCompletedInstallerBeforeFirstStart(t *testing.T) {
	m := newTestManager(t.Context(), t.TempDir())
	t.Cleanup(m.Close)

	docker := &startingConsoleDocker{checked: make(chan struct{}, 1), followed: make(chan string, 1)}
	m.Docker = docker
	d := runtimeFixture()

	state, err := m.load(d.ServerID)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(m.root(d.ServerID), 0700); err != nil {
		t.Fatal(err)
	}

	for _, attempt := range []struct {
		installed bool
		state     model.OperationState
		output    string
	}{
		{false, model.OperationFailed, "First attempt failed\n"},
		{true, model.OperationSucceeded, "Second attempt installed successfully\n"},
		{true, model.OperationSucceeded, "2026-10-20T18:14:12.123456789Z Installer completed\n"},
	} {
		state.Installed = attempt.installed
		state.Status.Operation = "install"

		state.Status.OperationState = attempt.state
		if err := writeAtomic(filepath.Join(m.root(d.ServerID), "installer.log"), []byte(attempt.output), 0600); err != nil {
			t.Fatal(err)
		}

		var output strings.Builder

		if err := m.StreamConsole(t.Context(), d, func(text string) error { output.WriteString(text); return nil }); err != nil {
			t.Fatal(err)
		}

		if output.String() != attempt.output {
			t.Fatal("console retained the previous attempt", output.String())
		}
	}
}

func TestBoundedOutputRetainsFailureAtEnd(t *testing.T) {
	output := &boundedOutput{}
	_, _ = output.Write([]byte(strings.Repeat("package output\n", 10000)))

	_, _ = output.Write([]byte("Fabric installer failed: missing server jar\n"))
	if output.Len() != 65536 || !strings.HasSuffix(output.String(), "Fabric installer failed: missing server jar\n") {
		t.Fatal("installer failure was truncated", output.Len())
	}
}

func TestConsoleProcess(_ *testing.T) {
	if os.Getenv("CAMPFIRE_CONSOLE_HELPER") != "1" {
		return
	}

	_, _ = os.Stdout.WriteString(strings.Repeat("continuous output\n", 20000) + "café 🎮\n")
	_, _ = os.Stderr.WriteString("stderr output\n")

	if os.Getenv("CAMPFIRE_CONSOLE_WAIT") == "1" {
		time.Sleep(30 * time.Second)
	}

	os.Exit(0)
}

func helperConsoleCommand(ctx context.Context, wait bool) *exec.Cmd {
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestConsoleProcess$")

	cmd.Env = append(os.Environ(), "CAMPFIRE_CONSOLE_HELPER=1")
	if wait {
		cmd.Env = append(cmd.Env, "CAMPFIRE_CONSOLE_WAIT=1")
	}

	return cmd
}

func TestConsoleStreamsBeyondSnapshotLimitAndCancels(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var output strings.Builder

	err := followCommand(ctx, helperConsoleCommand(ctx, false), func(text string) error {
		if len(text) > 8196 || !utf8.ValidString(text) {
			t.Error("invalid output chunk")
		}

		output.WriteString(text)

		return nil
	})
	if err != nil || !strings.Contains(output.String(), "café 🎮\n") || !strings.Contains(output.String(), "stderr output\n") || strings.Count(output.String(), "continuous output\n") != 20000 {
		t.Fatal("output lost", output.Len(), err)
	}

	ctx, stop := context.WithCancel(ctx)
	finished := make(chan error, 1)
	first := make(chan struct{}, 1)

	go func() {
		finished <- followCommand(ctx, helperConsoleCommand(ctx, true), func(string) error {
			select {
			case first <- struct{}{}:
			default:
			}

			return nil
		})
	}()

	select {
	case <-first:
	case <-time.After(time.Second):
		t.Fatal("console did not emit")
	}

	stop()

	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("cancelled console succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("cancel leaked subprocess")
	}

	sentinel := errors.New("consumer stopped")
	if err := followCommand(context.Background(), helperConsoleCommand(context.Background(), true), func(string) error { return sentinel }); !errors.Is(err, sentinel) {
		t.Fatal("consumer error lost", err)
	}
}
