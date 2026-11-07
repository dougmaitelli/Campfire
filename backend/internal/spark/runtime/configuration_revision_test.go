package runtime

import (
	"context"
	"errors"
	"io"
	"testing"

	"campfire/internal/shared/model"
)

type failingRestartDocker struct {
	*fakeDocker
	failStop bool
}

func (d *failingRestartDocker) Run(ctx context.Context, input io.Reader, args ...string) (string, error) {
	if d.failStop && args[0] == "stop" {
		return "", errors.New("stop failed")
	}

	return d.fakeDocker.Run(ctx, input, args...)
}

func TestPendingContainerSettingsSurviveSparkRestartAndFailedRestart(t *testing.T) {
	d := runtimeFixture()
	directory := t.TempDir()
	docker := &failingRestartDocker{fakeDocker: &fakeDocker{states: make(map[string]string)}}
	m := newTestManager(t.Context(), directory)
	m.Docker, m.Ownership = docker, func(string, string) error { return nil }

	awaitOperation(t, m, d, "install", "install")
	awaitOperation(t, m, d, "start", "start")

	// Emulate a container created before Spark recorded its active revision.
	e, err := m.load(d.ServerID)
	if err != nil {
		t.Fatal(err)
	}

	e.mu.Lock()
	e.Revision = ""
	e.mu.Unlock()

	if err := m.RememberContainerConfiguration(d); err != nil {
		t.Fatal(err)
	}

	d.Ember.Startup = "sleep 5000"

	status, err := m.Execute(t.Context(), d, model.RuntimeRequest{ServerID: d.ServerID, Action: model.ServerActionStatus})
	if err != nil || !status.RestartRequired {
		t.Fatal("container change did not request a restart", status, err)
	}

	m.Close()
	m = newTestManager(t.Context(), directory)
	m.Docker, m.Ownership = docker, func(string, string) error { return nil }
	t.Cleanup(m.Close)

	status, err = m.Execute(t.Context(), d, model.RuntimeRequest{ServerID: d.ServerID, Action: model.ServerActionStatus})
	if err != nil || !status.RestartRequired {
		t.Fatal("Spark restart lost the active container revision", status, err)
	}

	docker.failStop = true

	status = awaitOperation(t, m, d, "restart", "failed-restart")
	if status.OperationState != model.OperationFailed || !status.RestartRequired {
		t.Fatal("failed restart cleared pending settings", status)
	}

	docker.failStop = false

	status = awaitOperation(t, m, d, "restart", "successful-restart")
	if status.OperationState != model.OperationSucceeded || status.RestartRequired {
		t.Fatal("successful restart did not apply saved settings", status)
	}
}
