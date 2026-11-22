package service

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

func TestExecutionRejectionsAreClassifiedInOnePlace(t *testing.T) {
	for _, test := range []struct {
		name    string
		err     error
		state   model.ExecutionState
		message string
	}{
		{"unsupported commands", errConsoleCommandsUnsupported, model.ExecutionFailed, "This game does not support console commands."},
		{"not running", errServerNotRunning, model.ExecutionSkipped, "Server is not running; this schedule only runs while the server is running."},
		{"offline", errSparkOffline, model.ExecutionFailed, "Spark became unavailable before dispatch."},
		{"disabled", errSparkDisabled, model.ExecutionFailed, "Hosting Spark is disabled; execution was skipped."},
		{"busy", ErrServerActive, model.ExecutionFailed, "Server already has an active operation; this execution was skipped."},
		{"access denied", repository.ErrAccessDenied, model.ExecutionFailed, "Execution permission was revoked or the server was removed."},
		{"access missing", repository.ErrAccessNotFound, model.ExecutionFailed, "Execution permission was revoked or the server was removed."},
		{"server missing", ErrServerNotFound, model.ExecutionFailed, "Execution permission was revoked or the server was removed."},
		{"unknown", errors.New("connection lost"), "", ""},
		{"no error", nil, "", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, _, _, owner := activityFixture(t)
			item := savedSchedule(t, s, owner, model.ServerActionCommand)

			run, err := s.Store.ClaimSchedule(t.Context(), item.ID, time.Now(), nil)
			if err != nil {
				t.Fatal(err)
			}

			cause := test.err
			if cause != nil {
				cause = fmt.Errorf("wrapped: %w", cause)
			}

			handled, err := s.rejectAttempt(t.Context(), run, cause)
			if err != nil || handled != (test.state != "") {
				t.Fatal("incorrect rejection handling", handled, err)
			}

			final, err := s.Store.Run(t.Context(), run.ID)
			if err != nil {
				t.Fatal(err)
			}

			if !handled {
				if final.State != model.ExecutionDispatching || final.Message != "" {
					t.Fatal("unknown error modified execution", final)
				}
			} else if final.State != test.state || final.Message != test.message {
				t.Fatal("incorrect rejection result", final)
			}
		})
	}
}

func TestUnknownCommandDispatchFailureRemainsInterrupted(t *testing.T) {
	s, _, runtime, owner := activityFixture(t)
	item := savedSchedule(t, s, owner, model.ServerActionCommand)
	runtime.fail = true

	run, err := s.RunNow(t.Context(), owner, item.ID)
	if err != nil || run.State != model.ExecutionInterrupted || run.Message != "Dispatch failed; verify server state before retrying." {
		t.Fatal("unknown dispatch failure was treated as a definite rejection", run, err)
	}
}
