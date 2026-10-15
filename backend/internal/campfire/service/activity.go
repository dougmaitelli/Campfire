package service

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

type actorKey struct{}

var errScheduleChanged = errors.New("schedule changed before execution")
var errSparkOffline = errors.New("spark is offline before dispatch")
var errSparkDisabled = errors.New("spark is disabled before dispatch")

// WithActor binds authenticated identity to service mutations and their execution records.
func WithActor(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, actorKey{}, principal)
}

// ActivityService coordinates recurring work, retained execution history and audits.
type ActivityService struct {
	Store      *repository.ActivityRepository
	Retention  HistoryRetention
	deployment *DeploymentService
	auth       *AuthService
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	mu         sync.Mutex
	active     map[string]bool
}

// NewActivityService constructs the single-panel scheduler without starting background work.
func NewActivityService(store *repository.ActivityRepository, deployment *DeploymentService, auth *AuthService) *ActivityService {
	return &ActivityService{Store: store, deployment: deployment, auth: auth, active: make(map[string]bool)}
}

// Start recovers interrupted records and starts bounded background processing.
func (s *ActivityService) Start(ctx context.Context) error {
	if err := s.Retention.Validate(); err != nil {
		return err
	}

	if err := s.Store.Recover(ctx, time.Now()); err != nil {
		return err
	}

	ctx, s.cancel = context.WithCancel(ctx)
	if s.Retention.AuditDays > 0 || s.Retention.OperationDays > 0 {
		s.wg.Add(1)
		go s.maintainHistory(ctx)
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			if err := s.Store.EnqueueDue(ctx, time.Now()); err != nil && ctx.Err() == nil {
				slog.Error("queue recurring schedules", "error", err)
			}

			runs, err := s.Store.Pending(ctx)
			if err != nil && ctx.Err() == nil {
				slog.Error("read pending schedules", "error", err)
			}

			for _, run := range runs {
				s.mu.Lock()
				if s.active[run.ServerID] || len(s.active) >= 4 {
					s.mu.Unlock()
					continue
				}

				s.active[run.ServerID] = true
				s.mu.Unlock()

				s.wg.Add(1)
				go func() {
					defer s.wg.Done()
					defer func() { s.mu.Lock(); delete(s.active, run.ServerID); s.mu.Unlock() }()

					if err := s.process(ctx, run); err != nil && ctx.Err() == nil {
						slog.Error("process schedule execution", "run", run.ID, "error", err)
					}
				}()
			}

			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	return nil
}

// Close stops polling and waits for bounded dispatch requests, leaving recoverable state.
func (s *ActivityService) Close() {
	if s.cancel != nil {
		s.cancel()
		s.wg.Wait()
	}
}

func (s *ActivityService) principal(ctx context.Context, userID int64) (Principal, error) {
	user, err := s.auth.store.Access().User(ctx, userID)
	if err != nil {
		return Principal{}, err
	}

	if user.Disabled {
		return Principal{}, repository.ErrAccessDenied
	}

	role, err := s.auth.store.Access().Role(ctx, user.RoleID)
	if err != nil {
		return Principal{}, err
	}

	user.Role = role

	return PrincipalFromSession(database.UserSession{UserID: user.ID, User: user}), nil
}

func (s *ActivityService) authorize(ctx context.Context, principal Principal, serverID string, configure bool) error {
	permissions, err := s.auth.Access().ServerPermissions(ctx, principal, serverID)
	if err != nil {
		return err
	}

	if !slices.Contains(permissions, "server.view") {
		return ErrServerNotFound
	}

	if !slices.Contains(permissions, "server.operate") || configure && !slices.Contains(permissions, "server.configure") {
		return repository.ErrAccessDenied
	}

	return nil
}

// Save validates a recurrence and assigns its execution identity to the editing user.
func (s *ActivityService) Save(ctx context.Context, principal Principal, item database.Schedule, create bool) (database.Schedule, error) {
	if err := s.authorize(ctx, principal, item.ServerID, true); err != nil {
		return database.Schedule{}, err
	}

	if _, err := s.deployment.Servers.Get(ctx, item.ServerID); err != nil {
		return database.Schedule{}, err
	}

	if create {
		item.ID = rand.Text()
		item.Version = 1
	} else {
		old, err := s.Store.Schedule(ctx, item.ID)
		if err != nil {
			return database.Schedule{}, err
		}

		if old.ServerID != item.ServerID {
			return database.Schedule{}, repository.ErrAccessInput
		}

		item.CreatedAt, item.Version = old.CreatedAt, old.Version+1
		if item.Action == "command" && old.Action == "command" && item.Command == "" {
			item.Command = old.Command
		}
	}

	item.Name = strings.TrimSpace(item.Name)
	if item.Name == "" || utf8.RuneCountInString(item.Name) > 100 || strings.ContainsFunc(item.Name, unicode.IsControl) || item.IntervalMinutes < 1 || item.IntervalMinutes > 525600 || item.NextRunAt <= 0 || item.NextRunAt > time.Now().AddDate(5, 0, 0).Unix() {
		return database.Schedule{}, repository.ErrAccessInput
	}

	if !slices.Contains([]string{"start", "stop", "restart", "kill", "backup", "command"}, item.Action) {
		return database.Schedule{}, repository.ErrAccessInput
	}

	if item.Action == "command" {
		if strings.TrimSpace(item.Command) == "" || len(item.Command) > 4096 || strings.ContainsAny(item.Command, "\r\n\x00") {
			return database.Schedule{}, repository.ErrAccessInput
		}
	} else {
		item.Command = ""
	}

	item.UserID, item.Username = principal.UserID, principal.Username
	if err := s.Store.SaveSchedule(ctx, &item, create); err != nil {
		return database.Schedule{}, err
	}

	return item, nil
}

// Remove stops future recurrences while retaining execution and audit history.
func (s *ActivityService) Remove(ctx context.Context, principal Principal, id string) error {
	item, err := s.Store.Schedule(ctx, id)
	if err != nil {
		return err
	}

	if err := s.authorize(ctx, principal, item.ServerID, true); err != nil {
		return err
	}

	return s.Store.DeleteSchedule(ctx, id)
}

// Queue creates an explicit run; the worker rechecks permissions immediately before dispatch.
func (s *ActivityService) Queue(ctx context.Context, principal Principal, serverID, action, content, scheduleID, retryOf string) (database.ScheduleRun, error) {
	if err := s.authorize(ctx, principal, serverID, scheduleID != ""); err != nil {
		return database.ScheduleRun{}, err
	}

	server, err := s.deployment.Servers.Get(ctx, serverID)
	if err != nil {
		return database.ScheduleRun{}, err
	}

	run := database.ScheduleRun{ID: rand.Text(), ScheduleID: scheduleID, RetryOf: retryOf, ServerID: serverID, ServerName: server.Name, Action: action, Content: content, UserID: principal.UserID, Username: principal.Username, State: "queued", CreatedAt: time.Now().Unix()}
	if scheduleID != "" {
		item, err := s.Store.Schedule(ctx, scheduleID)
		if err != nil {
			return database.ScheduleRun{}, err
		}

		if item.ServerID != serverID || item.Action != action || item.Command != content {
			return database.ScheduleRun{}, repository.ErrAccessConflict
		}

		run.ScheduleVersion = item.Version
	}

	return run, s.Store.CreateRun(ctx, &run)
}

// RunNow queues a configured task under the current authorized caller.
func (s *ActivityService) RunNow(ctx context.Context, principal Principal, id string) (database.ScheduleRun, error) {
	item, err := s.Store.Schedule(ctx, id)
	if err != nil {
		return database.ScheduleRun{}, err
	}

	return s.Queue(ctx, principal, item.ServerID, item.Action, item.Command, item.ID, "")
}

// Retry creates a new tracked execution and never silently replays uncertain node work.
func (s *ActivityService) Retry(ctx context.Context, principal Principal, id string) (database.ScheduleRun, error) {
	old, err := s.Store.Run(ctx, id)
	if err != nil {
		return database.ScheduleRun{}, err
	}

	if err := s.authorize(ctx, principal, old.ServerID, false); err != nil {
		return database.ScheduleRun{}, err
	}

	if !slices.Contains([]string{"failed", "interrupted"}, old.State) || !slices.Contains([]string{"install", "start", "stop", "restart", "kill", "backup"}, old.Action) {
		return database.ScheduleRun{}, repository.ErrAccessInput
	}

	return s.Queue(ctx, principal, old.ServerID, old.Action, "", "", old.ID)
}

func (s *ActivityService) finish(ctx context.Context, run database.ScheduleRun, state, message string) error {
	return s.Store.FinishRun(ctx, run, state, message, time.Now())
}

func (s *ActivityService) checkQueued(ctx context.Context, run database.ScheduleRun) (Principal, error) {
	if run.ScheduleID != "" {
		item, err := s.Store.Schedule(ctx, run.ScheduleID)
		if errors.Is(err, repository.ErrAccessNotFound) {
			return Principal{}, errScheduleChanged
		}

		if err != nil {
			return Principal{}, err
		}

		if item.Version != run.ScheduleVersion || run.Recurring && !item.Enabled {
			return Principal{}, errScheduleChanged
		}
	}

	principal, err := s.principal(ctx, run.UserID)
	if err == nil {
		err = s.authorize(ctx, principal, run.ServerID, run.ScheduleID != "")
	}

	if err != nil {
		return principal, err
	}

	server, err := s.deployment.Servers.Get(ctx, run.ServerID)
	if err != nil {
		return principal, err
	}

	node, err := s.deployment.Sparks.Get(ctx, server.SparkID)
	if err != nil {
		return principal, err
	}

	if node.Disabled {
		return principal, errSparkDisabled
	}

	if s.deployment.Sparks.connectivity != nil && !node.Connected {
		return principal, errSparkOffline
	}

	return principal, err
}

func (s *ActivityService) rejectQueued(ctx context.Context, run database.ScheduleRun, err error) error {
	if errors.Is(err, errSparkOffline) {
		return s.Store.UpdateRun(ctx, run.ID, "queued", "Waiting for Spark to reconnect.", time.Now())
	}

	if errors.Is(err, errSparkDisabled) {
		return s.finish(ctx, run, "failed", "Hosting Spark is disabled; execution was skipped.")
	}

	if errors.Is(err, errScheduleChanged) {
		return s.finish(ctx, run, "cancelled", "Schedule was changed, paused or removed before execution.")
	}

	if !errors.Is(err, repository.ErrAccessDenied) && !errors.Is(err, repository.ErrAccessNotFound) && !errors.Is(err, ErrServerNotFound) {
		return err
	}

	if run.ScheduleID != "" {
		if err := s.Store.DisableSchedule(ctx, run.ScheduleID, run.ScheduleVersion); err != nil {
			return err
		}
	}

	return s.finish(ctx, run, "failed", "Execution permission was revoked or the server was removed.")
}

func (s *ActivityService) process(ctx context.Context, run database.ScheduleRun) error {
	if err := s.Store.TouchRun(ctx, run.ID); err != nil {
		return err
	}

	if run.State == "queued" {
		principal, err := s.checkQueued(ctx, run)
		if err != nil {
			return s.rejectQueued(ctx, run, err)
		}

		if err := s.Store.UpdateRun(ctx, run.ID, "dispatching", "", time.Now()); err != nil {
			return err
		}

		status, err := s.deployment.executeTracked(WithActor(ctx, principal), run)
		if errors.Is(err, ErrServerActive) {
			return s.finish(ctx, run, "failed", "Server already has an active operation; this execution was skipped.")
		}

		if err != nil {
			if errors.Is(err, errScheduleChanged) || errors.Is(err, errSparkOffline) || errors.Is(err, errSparkDisabled) || errors.Is(err, repository.ErrAccessDenied) || errors.Is(err, repository.ErrAccessNotFound) || errors.Is(err, ErrServerNotFound) {
				return s.rejectQueued(ctx, run, err)
			}

			return s.finish(ctx, run, "interrupted", "Dispatch failed; verify server state before retrying.")
		}

		if status.Busy && run.Action != "command" {
			if err := s.Store.UpdateRun(ctx, run.ID, "running", "", time.Now()); err != nil {
				return err
			}
		}

		return s.observe(ctx, run, status)
	}

	status, err := s.deployment.runAction(ctx, run.ServerID, "status", "")
	if err != nil {
		if time.Since(time.Unix(run.StartedAt, 0)) > 46*time.Minute {
			return s.finish(ctx, run, "interrupted", "Could not confirm the result; verify server state before retrying.")
		}

		return nil
	}

	return s.observe(ctx, run, status)
}

func (s *ActivityService) observe(ctx context.Context, run database.ScheduleRun, status model.RuntimeStatus) error {
	if run.Action == "command" {
		return s.finish(ctx, run, "succeeded", "")
	}

	if status.OperationID != run.ID {
		return s.finish(ctx, run, "interrupted", "The node reports a different operation; verify server state before retrying.")
	}

	if status.Error != "" {
		return s.finish(ctx, run, "failed", "Node operation failed; inspect server status and logs.")
	}

	if !status.Busy {
		return s.finish(ctx, run, "succeeded", "")
	}

	return nil
}

// BeginManual records a directly requested mutation before dispatch while preserving the API contract.
func (s *ActivityService) BeginManual(ctx context.Context, serverID, action, content string) (database.ScheduleRun, error) {
	principal, ok := ctx.Value(actorKey{}).(Principal)
	if !ok {
		return database.ScheduleRun{}, errors.New("execution requires an authenticated actor")
	}

	server, err := s.deployment.Servers.Get(ctx, serverID)
	if err != nil {
		return database.ScheduleRun{}, err
	}

	run := database.ScheduleRun{ID: rand.Text(), ServerID: serverID, ServerName: server.Name, UserID: principal.UserID, Username: principal.Username, Action: action, Content: content, State: "dispatching", CreatedAt: time.Now().Unix(), StartedAt: time.Now().Unix()}

	return run, s.Store.CreateRun(ctx, &run)
}
