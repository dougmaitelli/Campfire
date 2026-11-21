package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"
)

func setScheduledConsoleCommands(t *testing.T, s *ActivityService, enabled bool) {
	t.Helper()
	ember, err := s.deployment.Embers.Get(t.Context(), "ember")
	if err != nil {
		t.Fatal(err)
	}
	ember.ConsoleCommands = &enabled
	if _, err := s.deployment.Embers.Replace(t.Context(), ember.ID, ember); err != nil {
		t.Fatal(err)
	}
}

func TestUnsupportedCommandSchedulesCannotBeSavedOrEnabled(t *testing.T) {
	s, _, runtime, owner := activityFixture(t)
	item := savedSchedule(t, s, owner, model.ServerActionCommand)
	setScheduledConsoleCommands(t, s, false)

	input := item
	input.ID = ""
	if _, err := s.Save(t.Context(), owner, input, true); !errors.Is(err, errConsoleCommandsUnsupported) {
		t.Fatal("unsupported command schedule was created", err)
	}
	input = item
	input.Name = "Changed"
	if _, err := s.Save(t.Context(), owner, input, false); !errors.Is(err, errConsoleCommandsUnsupported) {
		t.Fatal("unsupported command schedule was updated", err)
	}
	if _, err := s.SetEnabled(t.Context(), owner, item.ID, false); err != nil {
		t.Fatal("unsupported schedule cannot be disabled", err)
	}
	if _, err := s.SetEnabled(t.Context(), owner, item.ID, true); !errors.Is(err, errConsoleCommandsUnsupported) {
		t.Fatal("unsupported schedule was enabled", err)
	}
	persisted, err := s.Store.Schedule(t.Context(), item.ID)
	if err != nil || persisted.Enabled || persisted.Name != item.Name {
		t.Fatal("rejected edits changed schedule", persisted, err)
	}
	// Other scheduled actions remain available.
	savedSchedule(t, s, owner, model.ServerActionStart)
	if runtime.calls != 0 {
		t.Fatal("schedule validation dispatched runtime work")
	}

	setScheduledConsoleCommands(t, s, true)
	if _, err := s.SetEnabled(t.Context(), owner, item.ID, true); err != nil {
		t.Fatal("supported schedule could not be re-enabled", err)
	}
}

func TestExistingCommandSchedulesFailClearlyWhenCapabilityChanges(t *testing.T) {
	for _, mode := range []string{"run now", "recurring"} {
		t.Run(mode, func(t *testing.T) {
			s, _, runtime, owner := activityFixture(t)
			item := savedSchedule(t, s, owner, model.ServerActionCommand)
			setScheduledConsoleCommands(t, s, false)
			var run database.ScheduleRun
			var err error
			if mode == "run now" {
				run, err = s.RunNow(t.Context(), owner, item.ID)
			} else {
				run, err = s.Store.ClaimSchedule(t.Context(), item.ID, time.Now(), nil)
				if err == nil {
					err = s.process(t.Context(), run)
				}
				if err == nil {
					run, err = s.Store.Run(t.Context(), run.ID)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if run.State != model.ExecutionFailed || run.Message != "This game does not support console commands." {
				t.Fatal("unsupported command was not recorded as a definite rejection", run)
			}
			if runtime.calls != 0 {
				t.Fatal("unsupported command reached Spark", runtime.calls)
			}
		})
	}
}

type consolePreflightSignal struct {
	checked chan struct{}
	once    sync.Once
}

func (s *consolePreflightSignal) Connected(string) bool {
	s.once.Do(func() { close(s.checked) })
	return true
}

func assertUnsupportedCommandRun(t *testing.T, s *ActivityService, runtime *activityRuntime, id string) {
	t.Helper()
	final, err := s.Store.Run(t.Context(), id)
	if err != nil || final.State != model.ExecutionFailed || final.Message != "This game does not support console commands." {
		t.Fatal("capability race was reported as interrupted dispatch", final, err)
	}
	if runtime.calls != 0 {
		t.Fatal("unsupported command reached Spark", runtime.calls)
	}
}

func TestConsoleCapabilityIsRecheckedAfterAcquiringExecutionLock(t *testing.T) {
	s, _, runtime, owner := activityFixture(t)
	item := savedSchedule(t, s, owner, model.ServerActionCommand)
	run, err := s.Store.ClaimSchedule(t.Context(), item.ID, time.Now(), nil)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	release, err := s.deployment.lockServer(ctx, run.ServerID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if release != nil {
			release()
		}
	}()

	// Let the initial preflight read the enabled capability, but prevent the
	// locked recheck until the real Ember definition has changed.
	signal := &consolePreflightSignal{checked: make(chan struct{})}
	s.deployment.Sparks.connectivity = signal
	done := make(chan error, 1)
	go func() { done <- s.process(ctx, run) }()
	select {
	case <-signal.checked:
	case <-ctx.Done():
		t.Fatal("initial preflight did not complete", ctx.Err())
	}
	setScheduledConsoleCommands(t, s, false)
	release()
	release = nil
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("execution did not finish", ctx.Err())
	}
	assertUnsupportedCommandRun(t, s, runtime, run.ID)
}

type consoleCapabilityStatusRuntime struct {
	*activityRuntime
	afterStatus func()
}

func (r *consoleCapabilityStatusRuntime) Action(ctx context.Context, node string, request model.RuntimeRequest) (model.RuntimeStatus, error) {
	status, err := r.activityRuntime.Action(ctx, node, request)
	if err == nil && request.Action == model.ServerActionStatus {
		r.afterStatus()
	}
	return status, err
}

func TestConsoleCapabilityIsRecheckedImmediatelyBeforeCommandDispatch(t *testing.T) {
	s, _, runtime, owner := activityFixture(t)
	item := savedSchedule(t, s, owner, model.ServerActionCommand)
	run, err := s.Store.ClaimSchedule(t.Context(), item.ID, time.Now(), nil)
	if err != nil {
		t.Fatal(err)
	}

	changed := false
	s.deployment.Remote = &consoleCapabilityStatusRuntime{
		activityRuntime: runtime,
		afterStatus: func() {
			setScheduledConsoleCommands(t, s, false)
			changed = true
		},
	}
	if err := s.process(t.Context(), run); err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("execution did not reach the runtime status check")
	}
	assertUnsupportedCommandRun(t, s, runtime, run.ID)
}
