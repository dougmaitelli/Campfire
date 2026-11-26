package runtime

import (
	"testing"
	"time"

	"campfire/internal/shared/model"
)

func TestCrashOOMAttributionRequiresRecentFatalExitAndNoStopIntent(t *testing.T) {
	for _, test := range []struct {
		name        string
		code        int
		oomAge      time.Duration
		intentional bool
		crash       bool
		reason      model.CrashReason
	}{
		{name: "successful exit after child OOM", code: 0, oomAge: time.Millisecond},
		{name: "intentional stop after child OOM", code: 137, oomAge: time.Millisecond, intentional: true},
		{name: "unrelated error after recent child OOM", code: 1, oomAge: time.Millisecond, crash: true, reason: model.CrashReasonUnknown},
		{name: "stale OOM before fatal exit", code: 137, oomAge: crashOOMExitWindow + time.Second, crash: true, reason: model.CrashReasonUnknown},
		{name: "inspect flag without OOM event", code: 137, crash: true, reason: model.CrashReasonUnknown},
		{name: "recent OOM followed by fatal exit", code: 137, oomAge: time.Millisecond, crash: true, reason: model.CrashReasonOutOfMemory},
	} {
		t.Run(test.name, func(t *testing.T) {
			m, observer, docker, d := crashFixture(t)
			now := time.Now()
			// Docker's flag may remain true after a child OOM, even on a clean exit.
			docker.oom = new(true)

			docker.finished = now
			if test.oomAge > 0 {
				if err := observer.observe(crashTestEvent(d.ServerID, "oom", now.Add(-test.oomAge), 0)); err != nil {
					t.Fatal(err)
				}
			}

			if test.intentional {
				docker.running = true

				if err := m.stop(t.Context(), d); err != nil {
					t.Fatal(err)
				}

				now = time.Now()
			}

			if err := observer.observe(crashTestEvent(d.ServerID, "die", now.Add(time.Nanosecond), test.code)); err != nil {
				t.Fatal(err)
			}

			records, failure := m.crashSnapshot(d.ServerID)
			if failure != "" {
				t.Fatal(failure)
			}

			if !test.crash {
				if len(records) != 0 {
					t.Fatal("non-crash was retained", records)
				}
			} else if len(records) != 1 || records[0].Reason != test.reason {
				t.Fatal(records)
			}
		})
	}
}

func TestCrashReplayClearsRecoveredStreamWarningWithoutDuplicatingEvidence(t *testing.T) {
	m, observer, _, d := crashFixture(t)

	event := crashTestEvent(d.ServerID, "die", time.Now(), 1)
	if err := observer.observe(event); err != nil {
		t.Fatal(err)
	}

	observer.mu.Lock()
	observer.failure = "Crash monitoring was interrupted; retained evidence may be incomplete."
	observer.mu.Unlock()

	if err := observer.observe(event); err != nil {
		t.Fatal(err)
	}

	records, failure := m.crashSnapshot(d.ServerID)
	if failure != "" || len(records) != 1 {
		t.Fatal("recovery health or deduplication failed", records, failure)
	}
}
