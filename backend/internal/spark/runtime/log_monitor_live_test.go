package runtime

import (
	"context"
	"crypto/rand"
	"os"
	"testing"
	"time"

	"campfire/internal/shared/model"
)

// This test creates only temporary containers; it needs no game-data quota mount.
func TestLiveDockerLogMonitor(t *testing.T) {
	if os.Getenv("CAMPFIRE_DOCKER_TEST") != "1" {
		t.Skip("set CAMPFIRE_DOCKER_TEST=1 to run against Docker")
	}

	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()

	docker := Docker{}
	d := runtimeFixture()
	d.ServerID = "log-live-" + rand.Text()
	d.Ember.LogAlerts = []model.LogAlert{{Title: "Problem", Message: "Review settings", Severity: model.LogError, Any: []model.LogMatch{{Value: "critical failure"}}}}
	name := container(d.ServerID)

	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()

		_, _ = docker.Run(cleanup, nil, "rm", "--force", name, name+"-install")
	})

	m := newTestManager(ctx, t.TempDir())
	t.Cleanup(m.Close)

	if err := m.WatchLogs(d); err != nil {
		t.Fatal(err)
	}

	if _, err := docker.Run(ctx, nil, "run", "--detach", "--name", name, "alpine:3.22", "sh", "-c", "echo Ready; echo critical failure; sleep 120"); err != nil {
		t.Fatal(err)
	}

	first := waitLogDetections(ctx, t, m, d.ServerID, 1, true)

	m.Close()
	restored := newTestManager(ctx, m.Directory)
	t.Cleanup(restored.Close)

	if err := restored.WatchLogs(d); err != nil {
		t.Fatal(err)
	}

	observation, ready, _ := restored.logSnapshot(d.ServerID)
	if !ready || len(observation.Alerts) != 1 || observation.Alerts[0].ID != first[0].ID {
		t.Fatal("Spark restart lost the Docker detection or readiness", observation)
	}

	if _, err := docker.Run(ctx, nil, "restart", "--time", "0", name); err != nil {
		t.Fatal(err)
	}

	status, err := restored.Execute(ctx, d, model.RuntimeRequest{ServerID: d.ServerID, Action: model.ServerActionStatus})
	if err != nil {
		t.Fatal(err)
	}

	if status.LogObservation.Executions[model.LogRuntime] == first[0].ExecutionID {
		t.Fatal("status reused the old Docker execution")
	}

	second := waitLogDetections(ctx, t, restored, d.ServerID, 2, true)
	if second[0].ID == second[1].ID || second[0].ExecutionID == second[1].ExecutionID {
		t.Fatal("Docker restart did not rearm detection", second)
	}

	// A failed installer can finish between inspections and is drained before rm.
	_, _ = docker.Run(ctx, nil, "run", "--name", name+"-install", "alpine:3.22", "sh", "-c", "echo critical failure; exit 1")

	restored.drainInstallerLogs(d.ServerID)

	third := waitLogDetections(ctx, t, restored, d.ServerID, 3, true)
	if third[2].Phase != model.LogInstallation {
		t.Fatal("installer failure was not detected", third)
	}
}

func waitLogDetections(ctx context.Context, t *testing.T, m *Manager, id string, count int, ready bool) []model.DetectedLogAlert {
	t.Helper()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		observation, actualReady, _ := m.logSnapshot(id)
		if observation != nil && len(observation.Alerts) == count && actualReady == ready {
			return observation.Alerts
		}

		select {
		case <-ctx.Done():
			t.Fatalf("log monitor did not converge: %+v, ready=%v: %v", observation, actualReady, ctx.Err())
		case <-ticker.C:
		}
	}
}
