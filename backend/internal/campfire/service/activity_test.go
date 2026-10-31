package service

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
	accessperm "campfire/internal/shared/permissions"
)

type activityRuntime struct {
	mu     sync.Mutex
	status model.RuntimeStatus
	calls  int
	fail   bool
}

type activityConnectivity struct{ online atomic.Bool }

func (c *activityConnectivity) Connected(string) bool { return c.online.Load() }

func (d *activityRuntime) Deploy(_ context.Context, _ string, input model.Deployment) (model.ConfigurationReceipt, error) {
	return model.ConfigurationReceipt{ServerID: input.ServerID}, nil
}
func (d *activityRuntime) Action(_ context.Context, _ string, input model.RuntimeRequest) (model.RuntimeStatus, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.fail {
		return model.RuntimeStatus{}, errors.New("connection lost")
	}

	if input.Action != "status" {
		d.calls++
		if d.status.Busy && d.status.Operation == "install" && (input.Action == "stop" || input.Action == "kill") {
			d.status.Busy = false
			d.status.ErrorCode = model.RuntimeOperationInterrupted
			d.status.Error = "Installation interrupted by request."

			return d.status, nil
		}

		d.status = model.RuntimeStatus{ServerID: input.ServerID, OperationID: input.OperationID, Operation: input.Action, State: "stopped", Busy: input.Action != "command"}
	}

	return d.status, nil
}

func activityFixture(t *testing.T) (*ActivityService, *database.Database, *activityRuntime, Principal) {
	t.Helper()

	ctx := context.Background()

	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	for _, item := range []any{
		&database.User{ID: 1, Username: "admin", CredentialVersion: 1, RoleID: database.AdminRole},
		&database.User{ID: 2, Username: "owner", CredentialVersion: 1, RoleID: database.UserRole},
		&database.User{ID: 3, Username: "operator", CredentialVersion: 1, RoleID: database.UserRole},
		&database.User{ID: 4, Username: "unrelated", CredentialVersion: 1, RoleID: database.UserRole},
		&database.Spark{ID: "node", Name: "Node", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
		&database.Ember{ID: "ember", Name: "Game", Definition: model.Ember{ID: "ember", Schema: 1, Name: "Game", Images: map[string]string{"default": "alpine:3.22"}, Startup: "echo ready", WorkDir: "/data", User: "1000"}},
		&database.Server{OwnerUserID: new(int64(2)), ID: "game", Name: "Game", SparkID: "node", EmberID: "ember", Image: "alpine:3.22", Variables: map[string]string{}, CPULimit: 1, MemoryMB: 128, DiskMB: 1024, Ports: []model.ServerPort{}, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
		&database.ServerAccessGrant{ServerID: "game", UserID: new(int64(3)), AccessRoleID: database.OperatorRole},
	} {
		if err := db.ORM().Omit("Role", "Spark", "Ember", "Server", "User").Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}

	auth := NewAuthService(repository.NewAuthRepository(db.ORM()))
	embers := NewEmberService(repository.NewEmberRepository(db.ORM()))
	runtime := &activityRuntime{status: model.RuntimeStatus{ServerID: "game", State: "stopped"}}
	deployment := &DeploymentService{Servers: NewServerService(repository.NewServerRepository(db.ORM()), embers), Sparks: NewSparkService(repository.NewSparkRepository(db.ORM()), nil), Embers: embers, Remote: runtime}
	deployment.History = NewResourceHistoryService(repository.NewResourceHistoryRepository(db.ORM()), deployment)
	activity := NewActivityService(repository.NewActivityRepository(db.ORM()), deployment, auth)
	deployment.Activity = activity

	principal, err := activity.principal(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}

	return activity, db, runtime, principal
}

func TestScheduledExecutionKeepsTokenScopeLimit(t *testing.T) {
	s, _, _, owner := activityFixture(t)
	limited := owner
	limited.APITokenID = 99
	limited.TokenScopes = accessperm.Expand([]string{accessperm.ServerSchedulesManage, accessperm.ServerBackupsCreate})

	item := savedSchedule(t, s, limited, "backup")
	if _, err := s.Save(t.Context(), limited, database.Schedule{ServerID: "game", Name: "Command", Action: "command", Command: "save-all", CronExpression: "0 3 * * *", TimeZone: "UTC"}, true); !errors.Is(err, repository.ErrAccessDenied) {
		t.Fatalf("token scheduled an action outside its scope: %v", err)
	}

	if _, err := s.RunNow(t.Context(), limited, item.ID); !errors.Is(err, repository.ErrAccessDenied) {
		t.Fatalf("manage scope granted immediate execution: %v", err)
	}

	run, err := s.Store.ClaimSchedule(t.Context(), item.ID, time.Now(), nil)
	if err != nil {
		t.Fatal(err)
	}

	principal, err := s.checkExecution(t.Context(), run)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.authorize(t.Context(), principal, "game", accessperm.ServerBackupsCreate); err != nil {
		t.Fatal(err)
	}

	for _, denied := range []string{accessperm.ServerConsoleWrite, accessperm.ServerFilesWrite, accessperm.ServerSchedulesRun} {
		if err := s.authorize(t.Context(), principal, "game", denied); !errors.Is(err, repository.ErrAccessDenied) {
			t.Fatalf("scheduled execution escaped token scope for %s: %v", denied, err)
		}
	}
	// Changing the persisted action must not turn the owner's full rights into
	// authorization for an action the scheduling token never held.
	run.Action = "command"
	if _, err := s.checkExecution(t.Context(), run); !errors.Is(err, repository.ErrAccessDenied) {
		t.Fatalf("execution accepted an action outside its original scope: %v", err)
	}
}

func TestStopInstallationRecordsInterruption(t *testing.T) {
	s, db, runtime, owner := activityFixture(t)
	ctx := context.WithValue(t.Context(), actorKey{}, owner)

	run, err := s.BeginManual(ctx, "game", "install", "")
	if err != nil {
		t.Fatal(err)
	}

	if err := s.Store.UpdateRun(ctx, run.ID, "running", "", time.Now()); err != nil {
		t.Fatal(err)
	}

	runtime.status = model.RuntimeStatus{ServerID: "game", OperationID: run.ID, Operation: "install", State: "installing", Busy: true}

	status, err := s.deployment.Action(ctx, "game", "stop")
	if err != nil || status.OperationID != run.ID || status.Busy {
		t.Fatalf("stop installation: %+v, %v", status, err)
	}

	saved, err := s.Store.Run(ctx, run.ID)
	if err != nil || saved.State != "interrupted" {
		t.Fatalf("installation history: %+v, %v", saved, err)
	}

	var count int64
	if err := db.ORM().Model(&database.ScheduleRun{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("stop should interrupt the existing execution: %d, %v", count, err)
	}
}

func savedSchedule(t *testing.T, s *ActivityService, p Principal, action string) database.Schedule {
	t.Helper()

	item, err := s.Save(context.Background(), p, database.Schedule{ServerID: "game", Name: "Nightly", Action: action, Command: "private-console-command", CronExpression: "0 3 * * *", TimeZone: "UTC", Enabled: true}, true)
	if err != nil {
		t.Fatal(err)
	}

	item.NextRunAt = time.Now().Add(-72 * time.Hour).Unix()

	if err := s.Store.SaveSchedule(t.Context(), &item, false); err != nil {
		t.Fatal(err)
	}

	return item
}

func TestCronSaveValidatesAndComputesNextRun(t *testing.T) {
	s, _, _, owner := activityFixture(t)
	for _, expression := range []string{"", "bad", "60 * * * *", "0 0 31 2 *"} {
		if _, err := s.Save(t.Context(), owner, database.Schedule{ServerID: "game", Name: "Invalid", Action: "restart", CronExpression: expression, TimeZone: "UTC"}, true); !errors.Is(err, repository.ErrAccessInput) {
			t.Fatalf("accepted %q: %v", expression, err)
		}
	}

	item, err := s.Save(t.Context(), owner, database.Schedule{ServerID: "game", Name: "Morning", Action: "restart", CronExpression: " 0  3 * * * ", TimeZone: "Asia/Tokyo", NextRunAt: 1, OnlyWhenRunning: true}, true)
	if err != nil {
		t.Fatal(err)
	}

	if item.CronExpression != "0 3 * * *" || item.NextRunAt <= time.Now().Unix() || !item.OnlyWhenRunning {
		t.Fatal("cron was not normalized and calculated", item)
	}

	next := time.Unix(item.NextRunAt, 0).UTC()
	if next.Hour() != 18 || next.Minute() != 0 {
		t.Fatal("ignored time zone", next)
	}
}

func TestScheduleRunningCondition(t *testing.T) {
	for _, test := range []struct {
		name, state, want string
		only              bool
		calls             int
	}{
		{"stopped skips", "stopped", "skipped", true, 0},
		{"running dispatches", "running", "succeeded", true, 1},
		{"unchecked dispatches while stopped", "stopped", "succeeded", false, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, db, runtime, owner := activityFixture(t)
			item := savedSchedule(t, s, owner, "command")
			item.OnlyWhenRunning = test.only

			if err := s.Store.SaveSchedule(t.Context(), &item, false); err != nil {
				t.Fatal(err)
			}

			runtime.status.State = model.RuntimeState(test.state)

			run, err := s.RunNow(t.Context(), owner, item.ID)
			if err != nil {
				t.Fatal(err)
			}

			if run.OnlyWhenRunning != test.only {
				t.Fatal("running condition was not captured in execution")
			}

			if err := s.process(t.Context(), run); err != nil {
				t.Fatal(err)
			}

			final, err := s.Store.Run(t.Context(), run.ID)
			if err != nil || final.State != test.want || runtime.calls != test.calls {
				t.Fatal("wrong conditional execution", final, runtime.calls, err)
			}

			if test.want == "skipped" {
				var audit database.AuditEvent
				if err := db.ORM().Where("target = ?", "server:game").Take(&audit).Error; err != nil || audit.Outcome != "skipped" {
					t.Fatal("skip was not audited", audit, err)
				}
			}
		})
	}
}

func TestActivityCursorPaginationAndDeletionRetention(t *testing.T) {
	s, db, _, owner := activityFixture(t)
	ctx := context.Background()
	now := time.Now().Unix()

	for i := range 105 {
		id := strings.Repeat("a", i+1)
		if err := s.Store.CreateRun(ctx, &database.ScheduleRun{ID: id, ServerID: "game", ServerName: "Game", State: "succeeded", CreatedAt: now}); err != nil {
			t.Fatal(err)
		}

		if err := s.Store.BeginAudit(ctx, &database.AuditEvent{ID: id, Action: "test", Target: "server:game", Outcome: "succeeded", CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}

	first, err := s.Store.Runs(ctx, owner.UserID, false, "", "")
	if err != nil || len(first) != 100 {
		t.Fatal(err, len(first))
	}

	second, err := s.Store.Runs(ctx, owner.UserID, false, "", first[99].ID)
	if err != nil || len(second) != 5 {
		t.Fatal("timestamp ties lost history", len(second), err)
	}

	audit, err := s.Store.Audit(ctx, "", "test", "")
	if err != nil || len(audit) != 100 {
		t.Fatal(err, len(audit))
	}

	audit, err = s.Store.Audit(ctx, audit[99].ID, "test", "")
	if err != nil || len(audit) != 5 {
		t.Fatal("audit pagination skipped ties", err, len(audit))
	}

	item := savedSchedule(t, s, owner, "backup")
	if err := db.ORM().Delete(&database.Server{}, "id = ?", "game").Error; err != nil {
		t.Fatal(err)
	}

	if _, err := s.Store.Schedule(ctx, item.ID); !errors.Is(err, repository.ErrAccessNotFound) {
		t.Fatal("server deletion retained recurring definition", err)
	}

	visible, err := s.Store.Runs(ctx, owner.UserID, false, "", "")
	if err != nil || len(visible) != 0 {
		t.Fatal("deleted server history exposed to regular user", err)
	}

	retained, err := s.Store.Runs(ctx, 1, true, "", "")
	if err != nil || len(retained) != 100 {
		t.Fatal("server deletion destroyed admin history", err)
	}
}

func TestDueSchedulesHaveNoRunsUntilClaimAndUseCurrentDefinition(t *testing.T) {
	s, db, runtime, owner := activityFixture(t)
	item := savedSchedule(t, s, owner, "command")
	now := time.Now()

	due, err := s.Store.Due(t.Context(), now)
	if err != nil || len(due) != 1 {
		t.Fatal(due, err)
	}

	var count int64
	if err := db.ORM().Model(&database.ScheduleRun{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("created history before an attempt", count, err)
	}

	item.Command = "new-command"
	if err := s.Store.SaveSchedule(t.Context(), &item, false); err != nil {
		t.Fatal(err)
	}

	run, err := s.Store.ClaimSchedule(t.Context(), item.ID, now, nil)
	if err != nil || run.Content != "new-command" || run.State != "dispatching" || run.StartedAt == 0 {
		t.Fatal("claim missed current configuration", run, err)
	}

	item.Command = "later-command"
	if err := s.Store.SaveSchedule(t.Context(), &item, false); err != nil {
		t.Fatal(err)
	}

	if err := s.Store.DeleteSchedule(t.Context(), item.ID); err != nil {
		t.Fatal(err)
	}

	if err := s.process(t.Context(), run); err != nil {
		t.Fatal(err)
	}

	saved, err := s.Store.Run(t.Context(), run.ID)
	if err != nil || saved.State != "succeeded" || runtime.calls != 1 {
		t.Fatal("editing or removing a schedule cancelled its started snapshot", saved, err)
	}

	body, err := json.Marshal(saved)
	if err != nil || strings.Contains(string(body), "new-command") || saved.Content != "" {
		t.Fatal("execution exposed private command", string(body), err)
	}
}

func TestScheduleClaimIsAtomicAndCoalescesMissedOccurrences(t *testing.T) {
	s, db, _, owner := activityFixture(t)
	item := savedSchedule(t, s, owner, "restart")
	now := time.Now()

	var wg sync.WaitGroup

	var successes atomic.Int64

	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()

			_, err := s.Store.ClaimSchedule(t.Context(), item.ID, now, nil)
			if err == nil {
				successes.Add(1)
			} else if !errors.Is(err, repository.ErrAccessConflict) {
				t.Error(err)
			}
		}()
	}

	wg.Wait()

	var count int64
	if err := db.ORM().Model(&database.ScheduleRun{}).Count(&count).Error; err != nil || count != 1 || successes.Load() != 1 {
		t.Fatal("duplicate attempt claims", count, successes.Load(), err)
	}

	current, err := s.Store.Schedule(t.Context(), item.ID)
	if err != nil || current.NextRunAt <= now.Unix() {
		t.Fatal("missed intervals were not coalesced", current, err)
	}
}

func TestOneOffScheduleConsumesOnlyItsStartedAttempt(t *testing.T) {
	s, _, _, owner := activityFixture(t)
	future := time.Now().Add(time.Hour).Unix()

	item, err := s.Save(t.Context(), owner, database.Schedule{ServerID: "game", Name: "Once", Action: "command", Command: "once-command", NextRunAt: future, Enabled: true}, true)
	if err != nil || item.CronExpression != "" || item.NextRunAt != future {
		t.Fatal("invalid one-off schedule", item, err)
	}

	if _, err := s.Store.ClaimSchedule(t.Context(), item.ID, time.Now(), nil); !errors.Is(err, repository.ErrAccessConflict) {
		t.Fatal("claimed future work", err)
	}

	run, err := s.Store.ClaimSchedule(t.Context(), item.ID, time.Unix(future, 0), nil)
	if err != nil || run.Recurring {
		t.Fatal(run, err)
	}

	current, err := s.Store.Schedule(t.Context(), item.ID)
	if err != nil || current.Enabled {
		t.Fatal("one-off remained enabled", current, err)
	}

	if _, err := s.Store.ClaimSchedule(t.Context(), item.ID, time.Unix(future+1, 0), nil); !errors.Is(err, repository.ErrAccessConflict) {
		t.Fatal("repeated one-off attempt", err)
	}

	if err := s.process(t.Context(), run); err != nil {
		t.Fatal(err)
	}

	current.NextRunAt = future + 3600

	current.Enabled = true
	if _, err := s.Save(t.Context(), owner, current, false); err != nil {
		t.Fatal("could not reschedule completed one-off", err)
	}
}

func TestOfflineSchedulesStayDefinitionsAndUseEditsOnReconnect(t *testing.T) {
	s, db, runtime, owner := activityFixture(t)
	connection := &activityConnectivity{}
	s.deployment.Sparks.connectivity = connection
	item := savedSchedule(t, s, owner, "command")

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}

	defer s.Close()

	time.Sleep(50 * time.Millisecond)

	var count int64
	if err := db.ORM().Model(&database.ScheduleRun{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("offline work created execution history", count, err)
	}

	item.Command = "updated-while-offline"
	if err := s.Store.SaveSchedule(ctx, &item, false); err != nil {
		t.Fatal(err)
	}

	connection.online.Store(true)

	deadline := time.Now().Add(4 * time.Second)

	for {
		runs, err := s.Store.Runs(ctx, owner.UserID, false, "game", "")
		if err != nil {
			t.Fatal(err)
		}

		if len(runs) == 1 && runs[0].State == "succeeded" {
			break
		}

		if time.Now().After(deadline) {
			t.Fatal("pending schedule did not execute after reconnection", runs)
		}

		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	s.Close()
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if runtime.calls != 1 {
		t.Fatal("unexpected dispatch count", runtime.calls)
	}
}

func TestAttemptRecoveryRetryAndPermissionChecks(t *testing.T) {
	s, db, runtime, owner := activityFixture(t)
	item := savedSchedule(t, s, owner, "restart")

	run, err := s.RunNow(t.Context(), owner, item.ID)
	if err != nil || run.State != "running" {
		t.Fatal(run, err)
	}

	if err := s.Store.Recover(t.Context(), time.Now()); err != nil {
		t.Fatal(err)
	}

	recovered, err := s.Store.Run(t.Context(), run.ID)
	if err != nil || recovered.State != "interrupted" {
		t.Fatal(recovered, err)
	}

	runtime.status.Busy = false

	retry, err := s.Retry(t.Context(), owner, run.ID)
	if err != nil || retry.ID == run.ID || retry.ScheduleID != item.ID || retry.RetryOf != run.ID || retry.State != "running" || runtime.calls != 2 {
		t.Fatal("explicit retry failed", retry, err)
	}

	operator, err := s.principal(t.Context(), 3)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.RunNow(t.Context(), operator, item.ID); !errors.Is(err, repository.ErrAccessDenied) {
		t.Fatal("operator configured a schedule", err)
	}

	if err := db.ORM().Model(&database.User{}).Where("id = ?", owner.UserID).Update("disabled", true).Error; err != nil {
		t.Fatal(err)
	}
	// Snapshots never bypass live authorization, even after the claim.
	if err := s.Store.FinishRun(t.Context(), retry, "failed", "test", time.Now()); err != nil {
		t.Fatal(err)
	}

	current, err := s.Store.Schedule(t.Context(), item.ID)
	if err != nil {
		t.Fatal(err)
	}

	claimed, err := s.Store.ClaimSchedule(t.Context(), item.ID, time.Unix(current.NextRunAt, 0), nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.process(t.Context(), claimed); err != nil {
		t.Fatal(err)
	}

	failed, err := s.Store.Run(t.Context(), claimed.ID)
	if err != nil || failed.State != "failed" || runtime.calls != 2 {
		t.Fatal("revoked identity dispatched work", failed, err)
	}
}
