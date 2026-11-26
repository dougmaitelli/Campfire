package runtime

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"
	"time"
)

type slowCrashDocker struct {
	*crashDockerFixture
	entered chan struct{}
	release chan struct{}
}

func (d *slowCrashDocker) Run(ctx context.Context, input io.Reader, args ...string) (string, error) {
	if args[0] == "logs" {
		close(d.entered)

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-d.release:
		}
	}

	return d.crashDockerFixture.Run(ctx, input, args...)
}

func TestCrashCaptureDoesNotBlockAnotherServersSnapshotOrStopIntent(t *testing.T) {
	m, observer, docker, d := crashFixture(t)
	docker.running = true
	slow := &slowCrashDocker{crashDockerFixture: docker, entered: make(chan struct{}), release: make(chan struct{})}
	m.Docker = slow

	captured := make(chan error, 1)
	go func() { captured <- observer.observe(crashTestEvent(d.ServerID, "die", time.Now(), 1)) }()

	defer func() {
		close(slow.release)

		if err := <-captured; err != nil {
			t.Error(err)
		}
	}()

	select {
	case <-slow.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("capture did not start")
	}

	ready := make(chan struct{})

	go func() {
		m.crashSnapshot("other")
		finish := m.expectCrashStop(t.Context(), "other")
		finish()
		close(ready)
	}()

	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Error("another server waited on slow Docker capture")
	}
}

func TestCrashJournalLockDoesNotBlockAnotherServer(t *testing.T) {
	m, observer, _, d := crashFixture(t)

	state, err := observer.state(d.ServerID)
	if err != nil {
		t.Fatal(err)
	}

	state.mu.Lock()
	// Loading, persisting and pruning this journal use this same lock.
	defer state.mu.Unlock()

	ready := make(chan struct{})

	go func() { m.crashSnapshot("other"); close(ready) }()

	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("journal lock leaked across servers")
	}
}

func TestRetiredCrashJournalCannotBeResavedByPendingWork(t *testing.T) {
	m, observer, _, d := crashFixture(t)
	if err := observer.observe(crashTestEvent(d.ServerID, "die", time.Now(), 1)); err != nil {
		t.Fatal(err)
	}

	state, err := observer.state(d.ServerID)
	if err != nil {
		t.Fatal(err)
	}

	if err := m.forgetCrashes(d.ServerID); err != nil {
		t.Fatal(err)
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	if !state.retired || len(state.Records) != 0 {
		t.Fatal("retirement retained sensitive evidence")
	}

	if err := observer.save(d.ServerID, state); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(observer.filename(d.ServerID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("pending work recreated retired journal", err)
	}
}
