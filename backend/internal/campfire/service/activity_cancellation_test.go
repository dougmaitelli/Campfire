package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

type cancelledDispatchRuntime struct {
	*activityRuntime
	cancel      context.CancelFunc
	acknowledge bool
}

func (d *cancelledDispatchRuntime) Action(ctx context.Context, sparkID string, input model.RuntimeRequest) (model.RuntimeStatus, error) {
	if input.Action == model.ServerActionStatus {
		return d.activityRuntime.Action(ctx, sparkID, input)
	}

	if d.acknowledge {
		status, err := d.activityRuntime.Action(ctx, sparkID, input)
		d.cancel()

		return status, err
	}

	d.cancel()

	return model.RuntimeStatus{}, ctx.Err()
}

func TestScheduleRequestCancellationReleasesClaim(t *testing.T) {
	for _, action := range []string{"run", "retry"} {
		for _, phase := range []string{"before-dispatch", "during-dispatch", "after-acknowledgement"} {
			t.Run(action+"/"+phase, func(t *testing.T) {
				s, db, runtime, owner := activityFixture(t)
				item := savedSchedule(t, s, owner, model.ServerActionBackup)

				old := database.ScheduleRun{ID: "failed-attempt", ScheduleID: item.ID, ServerID: item.ServerID, Action: item.Action, UserID: owner.UserID, State: model.ExecutionFailed}
				if action == "retry" {
					if err := s.Store.CreateRun(t.Context(), &old); err != nil {
						t.Fatal(err)
					}
				}

				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()

				if phase == "before-dispatch" {
					// ClaimSchedule has committed before process touches its record.
					err := db.ORM().Callback().Update().Before("gorm:begin_transaction").Register("test:cancel-dispatch", func(tx *gorm.DB) {
						if tx.Statement.Table == "schedule_runs" {
							cancel()
						}
					})
					if err != nil {
						t.Fatal(err)
					}
				} else {
					s.deployment.Remote = &cancelledDispatchRuntime{activityRuntime: runtime, cancel: cancel, acknowledge: phase == "after-acknowledgement"}
				}

				var err error
				if action == "retry" {
					_, err = s.Retry(ctx, owner, old.ID)
				} else {
					_, err = s.RunNow(ctx, owner, item.ID)
				}

				if !errors.Is(err, context.Canceled) {
					t.Fatalf("expected cancelled request, got %v", err)
				}

				var run database.ScheduleRun
				if err := db.ORM().Where("schedule_id = ? AND id <> ?", item.ID, old.ID).Take(&run).Error; err != nil {
					t.Fatal(err)
				}

				if run.State != model.ExecutionInterrupted || run.FinishedAt == 0 || run.Message == "" {
					t.Fatalf("cancelled dispatch was not finalized: %+v", run)
				}

				pending, err := s.Store.Pending(t.Context())
				if err != nil || len(pending) != 0 {
					t.Fatalf("uncertain dispatch became eligible for replay: %+v %v", pending, err)
				}

				wantCalls := 0
				if phase == "after-acknowledgement" {
					wantCalls = 1
				}

				if runtime.calls != wantCalls {
					t.Fatalf("unexpected dispatch count: %d, want %d", runtime.calls, wantCalls)
				}

				if _, err := s.Store.ClaimSchedule(t.Context(), item.ID, time.Now(), &owner.UserID); err != nil {
					t.Fatalf("cancelled request blocked the next execution: %v", err)
				}
			})
		}
	}
}
