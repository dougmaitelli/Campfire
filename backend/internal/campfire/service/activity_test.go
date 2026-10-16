package service

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

type activityRuntime struct {
	mu     sync.Mutex
	status model.RuntimeStatus
	calls  int
	fail   bool
}

type activityConnectivity struct{ online bool }

func (c *activityConnectivity) Connected(string) bool { return c.online }

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
		&database.User{ID: 1, Username: "admin", Version: 1, RoleID: database.AdminRole},
		&database.User{ID: 2, Username: "owner", Version: 1, RoleID: database.UserRole},
		&database.User{ID: 3, Username: "operator", Version: 1, RoleID: database.UserRole},
		&database.User{ID: 4, Username: "unrelated", Version: 1, RoleID: database.UserRole},
		&database.Spark{ID: "node", Name: "Node", Kind: "remote", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
		&database.Ember{ID: "ember", Name: "Game", Definition: model.Ember{ID: "ember", Schema: 1, Name: "Game", Images: map[string]string{"default": "alpine:3.22"}, Startup: "echo ready", WorkDir: "/data", User: "1000"}},
		&database.Server{ID: "game", Name: "Game", SparkID: "node", EmberID: "ember", Image: "alpine:3.22", Variables: map[string]string{}, CPULimit: 1, MemoryMB: 128, DiskMB: 1024, Ports: []model.ServerPort{}, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
		&database.ServerOwnership{ServerID: "game", UserID: 2},
		&database.ServerMember{ServerID: "game", UserID: 3, RoleID: database.OperatorRole},
	} {
		if err := db.ORM().Omit("Role", "Spark", "Ember", "Server", "User").Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}

	auth := NewAuthService(repository.NewAuthRepository(db.ORM()))
	embers := NewEmberService(repository.NewEmberRepository(db.ORM()))
	runtime := &activityRuntime{status: model.RuntimeStatus{ServerID: "game", State: "stopped"}}
	deployment := &DeploymentService{Servers: NewServerService(repository.NewServerRepository(db.ORM()), embers), Sparks: NewSparkService(repository.NewSparkRepository(db.ORM()), nil), Embers: embers, Remote: runtime}
	activity := NewActivityService(repository.NewActivityRepository(db.ORM()), deployment, auth)
	deployment.Activity = activity

	principal, err := activity.principal(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}

	return activity, db, runtime, principal
}

func savedSchedule(t *testing.T, s *ActivityService, p Principal, action string) database.Schedule {
	t.Helper()

	item, err := s.Save(context.Background(), p, database.Schedule{ServerID: "game", Name: "Nightly", Action: action, Command: "private-console-command", CronExpression: "0 3 * * *", TimeZone: "UTC", Enabled: true}, true)
	if err != nil {
		t.Fatal(err)
	}

	item.NextRunAt = time.Now().Add(-72 * time.Hour).Unix()

	item.Version++
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

			item.Version++
			if err := s.Store.SaveSchedule(t.Context(), &item, false); err != nil {
				t.Fatal(err)
			}

			runtime.status.State = test.state

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

func TestRecurringSchedulesCoalesceAndRecheckPermissions(t *testing.T) {
	s, _, runtime, owner := activityFixture(t)
	ctx := context.Background()
	item := savedSchedule(t, s, owner, "backup")
	now := time.Now()

	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()

			if err := s.Store.EnqueueDue(ctx, now); err != nil {
				t.Error(err)
			}
		}()
	}

	wg.Wait()

	runs, err := s.Store.Pending(ctx)
	if err != nil || len(runs) != 1 {
		t.Fatal("missed intervals duplicated work", len(runs), err)
	}

	updated, err := s.Store.Schedule(ctx, item.ID)
	if err != nil || updated.NextRunAt <= now.Unix() {
		t.Fatal("recurrence was not advanced", err)
	}

	if err := s.process(ctx, runs[0]); err != nil {
		t.Fatal(err)
	}

	run, err := s.Store.Run(ctx, runs[0].ID)
	if err != nil || run.State != "running" || runtime.calls != 1 || runtime.status.OperationID != run.ID {
		t.Fatal("operation ID not preserved", run.State, err)
	}

	runtime.mu.Lock()
	runtime.status.Busy = false
	runtime.mu.Unlock()

	if err := s.process(ctx, run); err != nil {
		t.Fatal(err)
	}

	if err := s.process(ctx, run); err != nil {
		t.Fatal(err)
	}

	final, err := s.Store.Run(ctx, run.ID)
	if err != nil || final.State != "succeeded" || final.FinishedAt == 0 {
		t.Fatal("result not retained", final, err)
	}

	events, err := s.Store.Audit(ctx, "", "server.backup", "")
	if err != nil || len(events) != 1 {
		t.Fatal("terminal audit duplicated", len(events), err)
	}

	item.NextRunAt = now.Unix()

	item, err = s.Save(ctx, owner, item, false)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.Store.EnqueueDue(ctx, time.Unix(item.NextRunAt, 0)); err != nil {
		t.Fatal(err)
	}

	if err := s.auth.store.Access().TransferOwner(ctx, "game", 1); err != nil {
		t.Fatal(err)
	}

	runs, err = s.Store.Pending(ctx)
	if err != nil || len(runs) != 1 {
		t.Fatal(err, len(runs))
	}

	if err := s.process(ctx, runs[0]); err != nil {
		t.Fatal(err)
	}

	updated, err = s.Store.Schedule(ctx, item.ID)
	if err != nil || updated.Enabled || runtime.calls != 1 {
		t.Fatal("revoked owner still executed", err, runtime.calls)
	}

	final, err = s.Store.Run(ctx, runs[0].ID)
	if err != nil || final.State != "failed" {
		t.Fatal("revoked run not recorded", err)
	}
}

func TestManualHistoryRecoveryAndExplicitRetry(t *testing.T) {
	s, _, runtime, owner := activityFixture(t)
	ctx := WithActor(context.Background(), owner)

	status, err := s.deployment.Action(ctx, "game", "start")
	if err != nil {
		t.Fatal(err)
	}

	run, err := s.Store.Run(ctx, status.OperationID)
	if err != nil || run.State != "running" {
		t.Fatal("manual operation not persisted", err)
	}

	runtime.mu.Lock()
	runtime.status.Busy = false
	runtime.mu.Unlock()

	if _, err := s.deployment.Action(ctx, "game", "stop"); err != nil {
		t.Fatal(err)
	}

	previous, err := s.Store.Run(ctx, run.ID)
	if err != nil || previous.State != "succeeded" {
		t.Fatal("new operation lost previous result", err, previous.State)
	}

	if err := s.Store.Recover(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}

	runs, err := s.Store.Runs(ctx, owner.UserID, false, "game", "")
	if err != nil || len(runs) != 2 {
		t.Fatal(err, len(runs))
	}

	var interrupted database.ScheduleRun

	for _, entry := range runs {
		if entry.State == "interrupted" {
			interrupted = entry
		}
	}

	if interrupted.ID == "" || runtime.calls != 2 {
		t.Fatal("uncertain mutation was replayed or lost")
	}

	retry, err := s.Retry(ctx, owner, interrupted.ID)
	if err != nil || retry.ID == interrupted.ID || retry.RetryOf != interrupted.ID || retry.State != "queued" {
		t.Fatal("retry did not create a new execution", err)
	}

	if _, err := s.Retry(ctx, owner, run.ID); err == nil {
		t.Fatal("successful run could be replayed")
	}

	runtime.mu.Lock()
	runtime.status.Busy = false
	runtime.mu.Unlock()

	if err := s.process(ctx, retry); err != nil {
		t.Fatal(err)
	}

	if runtime.calls != 3 {
		t.Fatal("explicit retry not dispatched")
	}
}

func TestSchedulesAuthorizationCancellationAndSecretRedaction(t *testing.T) {
	s, db, _, owner := activityFixture(t)
	ctx := context.Background()
	item := savedSchedule(t, s, owner, "command")

	operator, err := s.principal(ctx, 3)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.Save(ctx, operator, item, false); !errors.Is(err, repository.ErrAccessDenied) {
		t.Fatal("operator could edit schedules", err)
	}

	if _, err := s.RunNow(ctx, operator, item.ID); !errors.Is(err, repository.ErrAccessDenied) {
		t.Fatal("operator could trigger configured commands", err)
	}

	if _, err := s.Save(ctx, owner, database.Schedule{ServerID: "game", Name: "Bad", Action: "delete", CronExpression: "* * * * *", TimeZone: "UTC"}, true); err == nil {
		t.Fatal("destructive recurring action accepted")
	}

	items, err := s.Store.Schedules(ctx, 4, false, "")
	if err != nil || len(items) != 0 {
		t.Fatal("unrelated user saw schedules", err)
	}

	run, err := s.RunNow(ctx, owner, item.ID)
	if err != nil {
		t.Fatal(err)
	}

	encoded, err := json.Marshal(run)
	if err != nil || strings.Contains(string(encoded), "private-console-command") {
		t.Fatal("command leaked through execution DTO", err)
	}

	items, err = s.Store.Schedules(ctx, owner.UserID, false, "")
	if err != nil || len(items) != 1 || items[0].Command != "" {
		t.Fatal("schedule listing exposed a command", err)
	}

	if err := s.Remove(ctx, owner, item.ID); err != nil {
		t.Fatal(err)
	}

	if err := s.process(ctx, run); err != nil {
		t.Fatal(err)
	}

	final, err := s.Store.Run(ctx, run.ID)
	if err != nil || final.State != "cancelled" || final.Content != "" {
		t.Fatal("deleted schedule still executed or retained secret", err)
	}

	item = savedSchedule(t, s, owner, "restart")

	run, err = s.RunNow(ctx, owner, item.ID)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Model(&database.User{}).Where("id = 2").Update("disabled", true).Error; err != nil {
		t.Fatal(err)
	}

	if err := s.process(ctx, run); err != nil {
		t.Fatal(err)
	}

	final, err = s.Store.Run(ctx, run.ID)
	if err != nil || final.State != "failed" {
		t.Fatal("disabled identity executed", err)
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

func TestPausedSchedulesAndRevocationWhileWaitingForServer(t *testing.T) {
	s, _, runtime, owner := activityFixture(t)
	ctx := context.Background()

	item := savedSchedule(t, s, owner, "restart")
	if err := s.Store.EnqueueDue(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}

	runs, err := s.Store.Pending(ctx)
	if err != nil || len(runs) != 1 {
		t.Fatal(err, len(runs))
	}

	item.Enabled = false

	item, err = s.Save(ctx, owner, item, false)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.process(ctx, runs[0]); err != nil {
		t.Fatal(err)
	}

	final, err := s.Store.Run(ctx, runs[0].ID)
	if err != nil || final.State != "cancelled" || runtime.calls != 0 {
		t.Fatal("paused definition still executed", err)
	}

	manual, err := s.RunNow(ctx, owner, item.ID)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.process(ctx, manual); err != nil {
		t.Fatal(err)
	}

	if runtime.calls != 1 {
		t.Fatal("explicit run of paused schedule was not executed")
	}

	runtime.mu.Lock()
	runtime.status.Busy = false
	runtime.mu.Unlock()

	if err := s.process(ctx, database.ScheduleRun{ID: manual.ID, ServerID: "game", Action: "restart", State: "running"}); err != nil {
		t.Fatal(err)
	}

	queued, err := s.RunNow(ctx, owner, item.ID)
	if err != nil {
		t.Fatal(err)
	}

	release, err := s.deployment.lockServer(ctx, "game")
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- s.process(ctx, queued) }()

	deadline := time.Now().Add(time.Second)

	for {
		current, err := s.Store.Run(ctx, queued.ID)
		if err != nil {
			release()
			t.Fatal(err)
		}

		if current.State == "dispatching" {
			break
		}

		if time.Now().After(deadline) {
			release()
			t.Fatal("worker did not reach dispatch")
		}

		time.Sleep(time.Millisecond)
	}

	if err := s.auth.store.Access().TransferOwner(ctx, "game", 1); err != nil {
		release()
		t.Fatal(err)
	}

	release()

	if err := <-done; err != nil {
		t.Fatal(err)
	}

	final, err = s.Store.Run(ctx, queued.ID)
	if err != nil || final.State != "failed" || runtime.calls != 1 {
		t.Fatal("revocation while waiting did not block dispatch", final.State, err)
	}
}

func TestOfflineWorkWaitsAndCommandEditsPreserveSecrets(t *testing.T) {
	s, _, runtime, owner := activityFixture(t)
	ctx := context.Background()
	item := savedSchedule(t, s, owner, "command")
	item.Command = ""
	item.Enabled = false

	updated, err := s.Save(ctx, owner, item, false)
	if err != nil {
		t.Fatal(err)
	}

	stored, err := s.Store.Schedule(ctx, item.ID)
	if err != nil || stored.Command != "private-console-command" || stored.Enabled {
		t.Fatal("pausing erased the stored command", err)
	}

	connection := &activityConnectivity{}
	s.deployment.Sparks.connectivity = connection

	run, err := s.RunNow(ctx, owner, updated.ID)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.process(ctx, run); err != nil {
		t.Fatal(err)
	}

	current, err := s.Store.Run(ctx, run.ID)
	if err != nil || current.State != "queued" || current.StartedAt != 0 || runtime.calls != 0 {
		t.Fatal("offline work was lost or dispatched", current.State, err)
	}

	connection.online = true

	if err := s.process(ctx, current); err != nil {
		t.Fatal(err)
	}

	current, err = s.Store.Run(ctx, run.ID)
	if err != nil || current.State != "succeeded" || runtime.calls != 1 {
		t.Fatal("queued work did not resume", err)
	}
}
