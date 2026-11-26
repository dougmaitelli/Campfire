package runtime

import (
	"os"
	"testing"
	"time"
)

func TestCrashStorageFailureDoesNotBlockStopOrDiscardLiveEvidence(t *testing.T) {
	m, observer, docker, d := crashFixture(t)

	now := time.Now().Add(-time.Second)
	if err := observer.observe(crashTestEvent(d.ServerID, "start", now, 0)); err != nil {
		t.Fatal(err)
	}

	filename := observer.filename(d.ServerID)
	if err := os.Remove(filename); err != nil {
		t.Fatal(err)
	}

	if err := os.Mkdir(filename, 0700); err != nil {
		t.Fatal(err)
	}

	docker.running, docker.stopError = true, true

	if err := m.stop(t.Context(), d); err == nil {
		t.Fatal("expected Docker stop failure")
	}

	if err := observer.observe(crashTestEvent(d.ServerID, "die", time.Now(), 1)); err != nil {
		t.Fatal(err)
	}

	records, failure := m.crashSnapshot(d.ServerID)
	if len(records) != 1 || failure == "" {
		t.Fatal("failed storage masked crash or lost health warning", records, failure)
	}

	if err := os.Remove(filename); err != nil {
		t.Fatal(err)
	}

	observer.prune()
	observer.states = map[string]*crashState{}

	records, failure = m.crashSnapshot(d.ServerID)
	if len(records) != 1 || failure != "" {
		t.Fatal("retry did not persist bounded evidence", records, failure)
	}
}

func TestMalformedCrashStateDoesNotStopOtherServersObservation(t *testing.T) {
	m, observer, _, d := crashFixture(t)
	if err := os.WriteFile(observer.filename(d.ServerID), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	if err := observer.observe(crashTestEvent(d.ServerID, "die", now, 1)); err != nil {
		t.Fatal("one corrupt server stopped node monitoring", err)
	}

	e, err := m.load("other")
	if err != nil {
		t.Fatal(err)
	}

	e.Installed = true
	if err := m.persist("other", e); err != nil {
		t.Fatal(err)
	}

	if err := observer.observe(crashTestEvent("other", "die", now, 1)); err != nil {
		t.Fatal(err)
	}

	records, failure := m.crashSnapshot("other")
	if len(records) != 1 || failure != "" {
		t.Fatal(records, failure)
	}

	if _, failure := m.crashSnapshot(d.ServerID); failure == "" {
		t.Fatal("corrupt state was not reported")
	}
}
