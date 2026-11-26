package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"campfire/internal/shared/model"
)

type crashDockerFixture struct {
	mu        sync.Mutex
	running   bool
	stopError bool
	logs      string
	logsError bool
	oom       *bool
	finished  time.Time
	calls     [][]string
}

func (d *crashDockerFixture) Run(_ context.Context, _ io.Reader, args ...string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.calls = append(d.calls, append([]string(nil), args...))
	switch args[0] {
	case "inspect":
		switch args[2] {
		case "{{.State.Status}}":
			if d.running {
				return "running", nil
			}

			return "exited", nil
		case "{{.State.Running}}":
			return strconv.FormatBool(d.running), nil
		case "{{json .State}}":
			value, _ := json.Marshal(struct {
				StartedAt, FinishedAt time.Time
				OOMKilled             *bool
			}{d.finished.Add(-time.Minute), d.finished, d.oom})

			return string(value), nil
		default:
			return `{"id":"` + strings.Repeat("a", 64) + `","running":` + strconv.FormatBool(d.running) + `}`, nil
		}
	case "logs":
		if d.logsError {
			return "private Docker failure", errors.New("logs unavailable")
		}

		return d.logs, nil
	case "stop":
		if d.stopError {
			return "", errors.New("stop failed")
		}

		d.running = false
	}

	return "", nil
}

func crashFixture(t *testing.T) (*Manager, *crashMonitor, *crashDockerFixture, model.Deployment) {
	t.Helper()
	m, d := fileTestManager(t)

	e, err := m.load(d.ServerID)
	if err != nil {
		t.Fatal(err)
	}

	e.Installed = true
	if err := m.persist(d.ServerID, e); err != nil {
		t.Fatal(err)
	}

	docker := &crashDockerFixture{logs: "private console evidence\n"}
	m.Docker = docker
	observer := &crashMonitor{manager: m, states: map[string]*crashState{}, done: make(chan struct{})}
	close(observer.done)
	m.crashes = observer

	return m, observer, docker, d
}

func crashTestEvent(id, action string, at time.Time, code int) crashEvent {
	var event crashEvent

	event.Action, event.TimeNano = action, at.UnixNano()
	event.Actor.ID = strings.Repeat("a", 64)
	event.Actor.Attributes = map[string]string{"campfire.server": id, "name": container(id), "exitCode": strconv.Itoa(code)}

	return event
}

func TestCrashEventsPersistDeduplicateAndNeverInferOOMFrom137(t *testing.T) {
	m, observer, docker, d := crashFixture(t)
	now := time.Now().Add(-time.Second)

	docker.finished = now
	if err := observer.observe(crashTestEvent(d.ServerID, "start", now.Add(-time.Minute), 0)); err != nil {
		t.Fatal(err)
	}

	event := crashTestEvent(d.ServerID, "die", now.Add(time.Nanosecond), 137)
	for range 2 {
		if err := observer.observe(event); err != nil {
			t.Fatal(err)
		}
	}

	records, failure := m.crashSnapshot(d.ServerID)
	if failure != "" || len(records) != 1 || records[0].Reason != model.CrashReasonUnknown || records[0].ExitCode != 137 || records[0].Console != docker.logs {
		t.Fatal(records, failure)
	}

	info, err := os.Stat(observer.filename(d.ServerID))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal(info, err)
	}

	observer.states = map[string]*crashState{}

	records, failure = m.crashSnapshot(d.ServerID)
	if failure != "" || len(records) != 1 {
		t.Fatal("restart lost evidence", records, failure)
	}

	if err := observer.observe(event); err != nil {
		t.Fatal(err)
	}

	if records, _ = m.crashSnapshot(d.ServerID); len(records) != 1 {
		t.Fatal("replay duplicated crash", records)
	}
}

func TestCrashOOMEventsSurviveAutomaticRestartAndDoNotBleedIntoLaterExits(t *testing.T) {
	m, observer, docker, d := crashFixture(t)
	now := time.Now().Add(-time.Second)
	docker.finished = time.Time{} // Docker has already restarted; its exit flag is gone.

	for _, action := range []string{"start", "oom", "die"} {
		now = now.Add(time.Millisecond)
		if err := observer.observe(crashTestEvent(d.ServerID, action, now, 137)); err != nil {
			t.Fatal(err)
		}
	}
	// Even if a subsequent start event is unavailable, old OOM evidence must not
	// label a different exit as another out-of-memory kill.
	if err := observer.observe(crashTestEvent(d.ServerID, "die", now.Add(time.Millisecond), 137)); err != nil {
		t.Fatal(err)
	}

	records, _ := m.crashSnapshot(d.ServerID)
	if len(records) != 2 || records[0].Reason != model.CrashReasonOutOfMemory || records[1].Reason != model.CrashReasonUnknown {
		t.Fatal(records)
	}
}

func TestManagedStopIntentSurvivesRestartAndFailedStopDoesNotMaskCrash(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(strconv.FormatBool(failed), func(t *testing.T) {
			m, observer, docker, d := crashFixture(t)
			docker.running, docker.stopError = true, failed

			err := m.stop(t.Context(), d)
			if (err != nil) != failed {
				t.Fatal(err)
			}

			observer.states = map[string]*crashState{}
			if err := observer.observe(crashTestEvent(d.ServerID, "die", time.Now(), 137)); err != nil {
				t.Fatal(err)
			}

			records, _ := m.crashSnapshot(d.ServerID)
			if len(records) != map[bool]int{false: 0, true: 1}[failed] {
				t.Fatal("incorrect stop classification", records)
			}
		})
	}
}

func TestCrashCaptureBoundsEvidenceAndReportsUnavailableLogs(t *testing.T) {
	m, observer, docker, d := crashFixture(t)
	now := time.Now()
	docker.logs = strings.Repeat("é", model.CrashConsoleBytes) + "\xff"
	docker.finished = now
	docker.oom = new(false)

	if err := observer.observe(crashTestEvent(d.ServerID, "die", now.Add(time.Nanosecond), 1)); err != nil {
		t.Fatal(err)
	}

	records, _ := m.crashSnapshot(d.ServerID)
	if len(records) != 1 || !records[0].ConsoleTruncated || records[0].Validate() != nil || len(records[0].Console) > model.CrashConsoleBytes || records[0].Reason != model.CrashReasonUnknown {
		t.Fatal(records)
	}

	docker.logsError = true

	if err := observer.observe(crashTestEvent(d.ServerID, "die", now.Add(time.Second), 2)); err != nil {
		t.Fatal(err)
	}

	records, _ = m.crashSnapshot(d.ServerID)
	if len(records) != 2 || !records[1].ConsoleUnavailable || records[1].Console != "" {
		t.Fatal(records)
	}
}

func TestCrashMonitorIgnoresInstallersSuccessfulExitsAndDeletedServers(t *testing.T) {
	m, observer, _, d := crashFixture(t)
	now := time.Now()
	event := crashTestEvent(d.ServerID, "die", now, 1)

	event.Actor.Attributes["name"] += "-install"
	if err := observer.observe(event); err != nil {
		t.Fatal(err)
	}

	if err := observer.observe(crashTestEvent("unknown", "die", now, 1)); err != nil {
		t.Fatal(err)
	}

	if err := observer.observe(crashTestEvent(d.ServerID, "die", now, 0)); err != nil {
		t.Fatal(err)
	}

	e, err := m.load(d.ServerID)
	if err != nil {
		t.Fatal(err)
	}

	e.Status.State = model.RuntimeStateDeleting

	if err := observer.observe(crashTestEvent(d.ServerID, "die", now.Add(time.Second), 137)); err != nil {
		t.Fatal(err)
	}

	records, _ := m.crashSnapshot(d.ServerID)
	if len(records) != 0 {
		t.Fatal(records)
	}

	if err := m.forgetCrashes(d.ServerID); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(observer.filename(d.ServerID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(m.root(d.ServerID), "data")); err != nil {
		t.Fatal("crash cleanup touched game data", err)
	}
}

func TestCrashRetentionBoundsPrivateReplayAndPrunesWithoutNewCrashes(t *testing.T) {
	_, observer, _, d := crashFixture(t)

	now := time.Now()
	for index := range model.CrashReplayLimit + 3 {
		if err := observer.observe(crashTestEvent(d.ServerID, "die", now.Add(time.Duration(index)*time.Millisecond), 1)); err != nil {
			t.Fatal(err)
		}
	}

	state := observer.states[d.ServerID]
	if len(state.Records) != model.CrashReplayLimit {
		t.Fatal(len(state.Records))
	}

	state.Records[0].ExitedAt = now.Add(-model.CrashRetention - time.Second).Unix()

	observer.prune()

	if len(state.Records) != model.CrashReplayLimit-1 {
		t.Fatal("expired record retained", len(state.Records))
	}

	observer.states = map[string]*crashState{}

	restored, err := observer.state(d.ServerID)
	if err != nil || len(restored.Records) != model.CrashReplayLimit-1 {
		t.Fatal(restored, err)
	}
}
