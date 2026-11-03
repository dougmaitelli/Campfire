package runtime

import (
	"strings"
	"testing"
	"time"

	"campfire/internal/shared/model"
)

func TestMonitoredReadinessSurvivesLargeLogsRotationAndSparkRestart(t *testing.T) {
	for _, plain := range []bool{false, true} {
		name := "regex"
		if plain {
			name = "plain"
		}

		t.Run(name, func(t *testing.T) {
			m, docker, d := monitorFixture(t)
			if plain {
				d.Ember.Readiness, d.Ember.Ready = nil, "Ready"
				if err := m.WatchLogs(d); err != nil {
					t.Fatal(err)
				}

				select {
				case <-docker.attached:
				case <-time.After(3 * time.Second):
					t.Fatal("plain readiness monitor did not attach")
				}
			}

			status, err := m.Execute(t.Context(), d, model.RuntimeRequest{ServerID: d.ServerID, Action: model.ServerActionStatus})
			if err != nil || status.Ready {
				t.Fatal("server became ready without a matching log line", status, err)
			}

			noise := strings.Repeat("2026-01-01T00:00:01Z Player connected\n", 10000)
			docker.publish(t, noise+"2026-01-01T00:00:02Z \x1b[32mReady\x1b[0m\n"+noise)
			m.Close()

			docker.mu.Lock()
			docker.retained = "2026-01-01T00:00:03Z Rotated output\n"
			docker.mu.Unlock()

			restored := newTestManager(t.Context(), m.Directory)
			restored.Docker = docker
			t.Cleanup(restored.Close)

			if err := restored.WatchLogs(d); err != nil {
				t.Fatal(err)
			}

			select {
			case <-docker.attached:
			case <-time.After(3 * time.Second):
				t.Fatal("monitor did not resume after Spark restart")
			}

			status, err = restored.Execute(t.Context(), d, model.RuntimeRequest{ServerID: d.ServerID, Action: model.ServerActionStatus})
			if err != nil || !status.Ready {
				t.Fatal("rotation or Spark restart lost readiness", status, err)
			}
		})
	}
}

func TestMonitoredReadinessResetsForReplacementContainer(t *testing.T) {
	m, docker, d := monitorFixture(t)
	docker.publish(t, "2026-01-01T00:00:01Z Ready\n")

	docker.mu.Lock()
	docker.source.ID = "replacement-container"
	docker.mu.Unlock()

	status, err := m.Execute(t.Context(), d, model.RuntimeRequest{ServerID: d.ServerID, Action: model.ServerActionStatus})
	if err != nil || status.Ready {
		t.Fatal("replacement inherited the old container's readiness", status, err)
	}

	select {
	case <-docker.attached:
	case <-time.After(3 * time.Second):
		t.Fatal("monitor did not attach to the replacement")
	}

	docker.publish(t, "2026-01-01T00:00:02Z Ready\n")

	status, err = m.Execute(t.Context(), d, model.RuntimeRequest{ServerID: d.ServerID, Action: model.ServerActionStatus})
	if err != nil || !status.Ready {
		t.Fatal("replacement never became ready", status, err)
	}
}
