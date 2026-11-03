package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"campfire/internal/shared/model"
)

func TestLogMonitorRecoversCorruptCheckpoints(t *testing.T) {
	for name, contents := range map[string]string{
		"malformed":      "{",
		"invalid source": `{"sources":{"runtime":{"container_id":"container-one","started_at":"2026-01-01T00:00:00Z","execution_id":"invalid","cursor":"2026-01-01T00:00:00Z"}}}`,
		"oversized":      strings.Repeat("x", (1<<20)+1),
	} {
		t.Run(name, func(t *testing.T) {
			manager, docker, deployment := monitorFixture(t)
			manager.stopLogMonitor(deployment.ServerID)

			filename := filepath.Join(manager.root(deployment.ServerID), "log-state.json")
			if err := writeAtomic(filename, []byte(contents), 0600); err != nil {
				t.Fatal(err)
			}

			docker.mu.Lock()
			docker.retained = "2026-01-01T00:00:01Z Ready\n2026-01-01T00:00:02Z critical failure\n"
			docker.mu.Unlock()

			if err := manager.WatchLogs(deployment); err != nil {
				t.Fatal("corrupt cache blocked monitoring recovery", err)
			}

			select {
			case <-docker.attached:
			case <-time.After(3 * time.Second):
				t.Fatal("monitor did not replay retained logs")
			}

			observation, ready, _ := manager.logSnapshot(deployment.ServerID)
			if !ready || len(observation.Alerts) != 1 || !strings.Contains(observation.Error, "quarantined") {
				t.Fatal("recovery lost readiness, detection, or warning", observation, ready)
			}

			files, err := filepath.Glob(filename + ".corrupt-*")
			if err != nil || len(files) != 1 {
				t.Fatal("invalid checkpoint was not quarantined", files, err)
			}

			saved, err := os.ReadFile(files[0])
			if err != nil || string(saved) != contents {
				t.Fatal("quarantined checkpoint was changed", err)
			}
		})
	}
}

type monitoredDocker struct {
	*fakeDocker
	mu            sync.Mutex
	source        logContainer
	retained      string
	emit          func(string) error
	attached      chan string
	followers     int
	maximum       int
	installer     logContainer
	installerLogs string
}

func (d *monitoredDocker) inspectLogContainer(_ context.Context, name string) (logContainer, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if strings.HasSuffix(name, "-install") {
		return d.installer, nil
	}

	return d.source, nil
}

func (d *monitoredDocker) followLogOutput(ctx context.Context, name string, since string, emit func(string) error) error {
	d.mu.Lock()
	if strings.HasSuffix(name, "-install") {
		logs := d.installerLogs
		d.mu.Unlock()

		return emit(logs)
	}

	d.emit = emit
	d.followers++
	d.maximum = max(d.maximum, d.followers)
	retained := d.retained

	d.mu.Unlock()
	defer func() { d.mu.Lock(); d.followers--; d.mu.Unlock() }()

	if retained != "" {
		if err := emit(retained); err != nil {
			return err
		}
	}

	select {
	case d.attached <- since:
	case <-ctx.Done():
		return ctx.Err()
	}

	<-ctx.Done()

	return ctx.Err()
}

func (d *monitoredDocker) publish(t *testing.T, text string) {
	t.Helper()
	d.mu.Lock()
	emit := d.emit
	d.mu.Unlock()

	if emit == nil {
		t.Fatal("monitor did not attach")
	}

	if err := emit(text); err != nil {
		t.Fatal(err)
	}
}

func monitorFixture(t *testing.T) (*Manager, *monitoredDocker, model.Deployment) {
	t.Helper()

	d := runtimeFixture()
	d.Ember.Readiness = &model.Readiness{Any: []model.LogMatch{{Value: "^Ready$", Regex: true}}}
	d.Ember.Ready = ""
	d.Ember.LogAlerts = []model.LogAlert{{Title: "Problem", Message: "Check settings", Severity: model.LogError, Phase: model.LogRuntime, Any: []model.LogMatch{{Value: "critical failure", IgnoreCase: true}}}}
	docker := &monitoredDocker{fakeDocker: &fakeDocker{states: map[string]string{container(d.ServerID): "running"}}, source: logContainer{ID: "container-one", StartedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Running: true, Status: "running"}, attached: make(chan string, 16)}
	m := newTestManager(t.Context(), filepath.Join(t.TempDir(), "runtime"))
	m.Docker = docker
	t.Cleanup(m.Close)

	if err := m.WatchLogs(d); err != nil {
		t.Fatal(err)
	}

	select {
	case <-docker.attached:
	case <-time.After(3 * time.Second):
		t.Fatal("monitor did not start")
	}

	return m, docker, d
}

func TestLogMonitorSharesReadinessAndErrorsAcrossChunksAndRestarts(t *testing.T) {
	m, docker, d := monitorFixture(t)
	docker.publish(t, "2026-01-01T00:00:01Z \x1b[32mRe")
	docker.publish(t, "ady\x1b[0m\n2026-01-01T00:00:02Z CRITICAL fai")
	docker.publish(t, "lure token=private\n")

	observation, ready, ok := m.logSnapshot(d.ServerID)
	if !ok || !ready || len(observation.Alerts) != 1 {
		t.Fatalf("shared matcher lost output: %+v, ready=%v", observation, ready)
	}

	firstID := observation.Alerts[0].ID

	docker.publish(t, "2026-01-01T00:00:03Z critical failure again\n")

	observation, _, _ = m.logSnapshot(d.ServerID)
	if len(observation.Alerts) != 1 {
		t.Fatal("repeated matches created duplicate issues")
	}

	if err := m.WatchLogs(d); err != nil {
		t.Fatal(err)
	}

	docker.mu.Lock()
	maximum := docker.maximum
	docker.mu.Unlock()

	if maximum != 1 {
		t.Fatal("configuration checks started duplicate monitors")
	}

	m.Close()
	restored := newTestManager(t.Context(), m.Directory)
	restored.Docker = docker
	t.Cleanup(restored.Close)

	if err := restored.WatchLogs(d); err != nil {
		t.Fatal(err)
	}

	select {
	case since := <-docker.attached:
		if since != "2026-01-01T00:00:03Z" {
			t.Fatal("cursor did not survive restart", since)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("monitor did not reconnect")
	}

	observation, ready, _ = restored.logSnapshot(d.ServerID)
	if !ready || len(observation.Alerts) != 1 || observation.Alerts[0].ID != firstID {
		t.Fatal("restart forgot readiness or issue identity", observation)
	}

	docker.mu.Lock()
	docker.source.StartedAt = docker.source.StartedAt.Add(time.Minute)
	docker.mu.Unlock()

	status, err := restored.Execute(t.Context(), d, model.RuntimeRequest{ServerID: d.ServerID, Action: model.ServerActionStatus})
	if err != nil || status.Ready {
		t.Fatal("status reused readiness before the stream detected the new run", status, err)
	}

	select {
	case <-docker.attached:
	case <-time.After(3 * time.Second):
		t.Fatal("new container execution did not rearm monitoring")
	}

	_, ready, _ = restored.logSnapshot(d.ServerID)
	if ready {
		t.Fatal("new execution reused previous readiness")
	}

	docker.publish(t, "2026-01-01T00:01:01Z critical failure\n")

	observation, _, _ = restored.logSnapshot(d.ServerID)
	if len(observation.Alerts) != 2 || observation.Alerts[1].ID == firstID {
		t.Fatal("new execution did not create a fresh occurrence", observation)
	}
}

func TestLogMonitorBoundsLinesAndRetainsErrorsAfterExit(t *testing.T) {
	m, docker, d := monitorFixture(t)
	docker.publish(t, "2026-01-01T00:00:01Z "+strings.Repeat("x", logLineLimit)+"\n2026-01-01T00:00:02Z critical failure\n")
	docker.mu.Lock()
	docker.source.Running = false
	docker.source.Status = "exited"
	docker.mu.Unlock()

	observation, _, _ := m.logSnapshot(d.ServerID)
	if len(observation.Alerts) != 1 || observation.Error == "" {
		t.Fatal("oversized output was not bounded and reported", observation)
	}

	status, err := m.Execute(t.Context(), d, model.RuntimeRequest{ServerID: d.ServerID, Action: model.ServerActionStatus})
	if err != nil || status.LogObservation == nil || len(status.LogObservation.Alerts) != 1 {
		t.Fatal("status did not report persistent matches", status, err)
	}
}

func TestLogMonitorDrainsFastInstallerBeforeRemoval(t *testing.T) {
	m, docker, d := monitorFixture(t)

	d.Ember.LogAlerts[0].Phase = model.LogInstallation
	if err := m.WatchLogs(d); err != nil {
		t.Fatal(err)
	}

	docker.mu.Lock()
	docker.installer = logContainer{ID: "fast-installer", StartedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Status: "exited"}
	docker.installerLogs = "2026-01-01T00:00:01Z critical failure\n"
	docker.mu.Unlock()
	m.drainInstallerLogs(d.ServerID)
	m.drainInstallerLogs(d.ServerID)
	docker.mu.Lock()
	docker.installer = logContainer{Status: "missing"}
	docker.mu.Unlock()

	observation, _, _ := m.logSnapshot(d.ServerID)
	if len(observation.Alerts) != 1 || observation.Alerts[0].Phase != model.LogInstallation {
		t.Fatal("fast installer was missed or duplicated", observation)
	}
}

func TestLogMonitorUsesDeployedDefinitionAndSkipsDeletedServers(t *testing.T) {
	m, docker, original := monitorFixture(t)
	updated := original
	updated.Ember.LogAlerts = append([]model.LogAlert{}, original.Ember.LogAlerts...)

	updated.Ember.LogAlerts[0].Any = []model.LogMatch{{Value: "new failure"}}
	if err := m.WatchLogs(updated); err != nil {
		t.Fatal(err)
	}

	select {
	case <-docker.attached:
	case <-time.After(3 * time.Second):
		t.Fatal("updated monitor did not start")
	}

	if _, err := m.Execute(t.Context(), original, model.RuntimeRequest{ServerID: original.ServerID, Action: model.ServerActionStatus}); err != nil {
		t.Fatal(err)
	}

	docker.publish(t, "2026-01-01T00:00:01Z critical failure\n2026-01-01T00:00:02Z new failure\n")

	observation, _, _ := m.logSnapshot(original.ServerID)
	if len(observation.Alerts) != 1 || observation.Alerts[0].RuleKey != updated.Ember.LogAlerts[0].Key() {
		t.Fatal("stale request replaced the deployed rules", observation)
	}

	m.stopLogMonitor(original.ServerID)

	entry, err := m.load(original.ServerID)
	if err != nil {
		t.Fatal(err)
	}

	entry.mu.Lock()
	entry.Status.State = model.RuntimeStateDeleted
	entry.mu.Unlock()

	if err := m.WatchLogs(updated); err != nil {
		t.Fatal(err)
	}

	if _, _, monitored := m.logSnapshot(original.ServerID); monitored {
		t.Fatal("deleted server restarted its monitor")
	}
}
