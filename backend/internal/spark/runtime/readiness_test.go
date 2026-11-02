package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"campfire/internal/shared/limits"
	"campfire/internal/shared/model"
)

type readinessDocker struct {
	*fakeDocker
	id       string
	started  string
	logs     string
	logError error
	logCalls int
	logArgs  []string
}

func (d *readinessDocker) Run(ctx context.Context, input io.Reader, args ...string) (string, error) {
	if args[0] == "logs" {
		d.logCalls++
		d.logArgs = slices.Clone(args)

		return d.logs, d.logError
	}

	out, err := d.fakeDocker.Run(ctx, input, args...)
	if err != nil || len(args) < 3 || args[2] != containerStateFormat {
		return out, err
	}

	var state containerState
	if err := json.Unmarshal([]byte(out), &state); err != nil {
		return "", err
	}

	state.ID, state.State.StartedAt = d.id, d.started
	data, err := json.Marshal(state)

	return string(data), err
}

func readinessStatus(t *testing.T, m *Manager, d model.Deployment) model.RuntimeStatus {
	t.Helper()

	status, err := m.Execute(t.Context(), d, model.RuntimeRequest{ServerID: d.ServerID, Action: model.ServerActionStatus})
	if err != nil {
		t.Fatal(err)
	}

	return status
}

func TestReadinessSurvivesLogRotationAndSparkRestart(t *testing.T) {
	d := runtimeFixture()
	directory := t.TempDir()
	docker := &readinessDocker{
		fakeDocker: &fakeDocker{states: map[string]string{container(d.ServerID): "running"}},
		id:         "container-a",
		started:    "2026-01-01T00:00:00Z",
		logs:       "Booting\n",
	}
	m := newTestManager(t.Context(), directory)
	t.Cleanup(m.Close)

	m.Docker = docker
	if status := readinessStatus(t, m, d); status.State != model.RuntimeStateRunning || status.Ready {
		t.Fatal("server became ready before its startup message", status)
	}

	// Startup completes after the lifecycle operation, then produces enough
	// output to put the readiness message outside the former 200-line tail.
	docker.logs = "Ready\n" + strings.Repeat("Player connected\n", 300)

	if status := readinessStatus(t, m, d); !status.Ready {
		t.Fatal("late readiness message was missed", status)
	}

	wantArgs := []string{"logs", "--since", docker.started, container(d.ServerID)}
	if !slices.Equal(docker.logArgs, wantArgs) {
		t.Fatal("readiness must search only the current container run", docker.logArgs)
	}

	calls := docker.logCalls

	docker.logs, docker.logError = "Rotated output\n", errors.New("logs unavailable")
	if status := readinessStatus(t, m, d); !status.Ready || docker.logCalls != calls {
		t.Fatal("ready server depended on retaining its startup log", status, docker.logCalls)
	}

	m.Close()
	m = newTestManager(t.Context(), directory)
	t.Cleanup(m.Close)

	m.Docker = docker
	if status := readinessStatus(t, m, d); !status.Ready || docker.logCalls != calls {
		t.Fatal("Spark restart lost persisted readiness", status, docker.logCalls)
	}
}

func TestReadinessResetsForNewContainerRun(t *testing.T) {
	for _, change := range []string{"restart", "replacement"} {
		t.Run(change, func(t *testing.T) {
			d := runtimeFixture()
			docker := &readinessDocker{
				fakeDocker: &fakeDocker{states: map[string]string{container(d.ServerID): "running"}},
				id:         "container-a",
				started:    "2026-01-01T00:00:00Z",
				logs:       "Ready\n",
			}
			m := newTestManager(t.Context(), t.TempDir())
			t.Cleanup(m.Close)

			m.Docker = docker
			if status := readinessStatus(t, m, d); !status.Ready {
				t.Fatal(status)
			}

			if change == "restart" {
				docker.started = "2026-01-01T01:00:00Z"
			} else {
				docker.id = "container-b"
			}

			docker.logs = "Booting\n"

			if status := readinessStatus(t, m, d); status.Ready {
				t.Fatal("new container run inherited readiness", status)
			}

			if !slices.Contains(docker.logArgs, docker.started) {
				t.Fatal("readiness included logs from a previous run", docker.logArgs)
			}

			docker.logs = "Ready\n"

			if status := readinessStatus(t, m, d); !status.Ready {
				t.Fatal("new run never became ready", status)
			}

			docker.states[container(d.ServerID)] = "exited"
			if status := readinessStatus(t, m, d); status.Ready || status.State != model.RuntimeStateStopped {
				t.Fatal("stopped container remained ready", status)
			}
		})
	}
}

func TestReadinessLogFailureDoesNotPersistReadiness(t *testing.T) {
	d := runtimeFixture()
	docker := &readinessDocker{
		fakeDocker: &fakeDocker{states: map[string]string{container(d.ServerID): "running"}},
		id:         "container-a",
		started:    "2026-01-01T00:00:00Z",
		logs:       "Ready\n",
		logError:   errors.New("read failed"),
	}
	m := newTestManager(t.Context(), t.TempDir())
	t.Cleanup(m.Close)

	m.Docker = docker
	if status := readinessStatus(t, m, d); status.Ready {
		t.Fatal("failed log read marked server ready", status)
	}

	docker.logs, docker.logError = "Booting\n", nil

	if status := readinessStatus(t, m, d); status.Ready {
		t.Fatal("failed log read cached readiness", status)
	}
}

func TestReadinessOutputSearchesBeyondCaptureLimit(t *testing.T) {
	for _, ember := range []model.RuntimeDefinition{
		{Ready: "Ready for players"},
		{Readiness: &model.Readiness{StripANSI: true, Any: []model.ReadinessMatch{{Value: "^Ready for players$", Regex: true}}}},
	} {
		for _, chunkSize := range []int{1, 7, 32 * 1024, 4 * limits.LogCaptureBytes} {
			output := &readinessOutput{ember: ember}

			logs := strings.Repeat("Startup output\n", 10000) + "\x1b[32mReady for players\x1b[0m\n" + strings.Repeat("Player connected\n", 10000)
			if chunkSize < 1024 {
				logs = "Booting\n\x1b[32mReady for players\x1b[0m\n"
			}

			for len(logs) > 0 {
				size := min(len(logs), chunkSize)
				if n, err := output.Write([]byte(logs[:size])); n != size || err != nil {
					t.Fatal("readiness log capture failed", n, err)
				}

				logs = logs[size:]
			}

			if !output.ready || output.buffer.Len() > limits.LogCaptureBytes {
				t.Fatal("readiness message was truncated or memory was unbounded", output.ready, output.buffer.Len(), chunkSize)
			}
		}
	}
}
