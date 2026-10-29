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
	"campfire/internal/campfire/recurrence"
	"campfire/internal/shared/model"
	accessperm "campfire/internal/shared/permissions"
)

type actorKey struct{}

var errSparkOffline = errors.New("spark is offline before dispatch")
var errSparkDisabled = errors.New("spark is disabled before dispatch")
var errServerNotRunning = errors.New("schedule requires a running server")

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

		var after *database.Schedule

		for {
			runs, err := s.Store.Pending(ctx)
			if err != nil && ctx.Err() == nil {
				slog.Error("read active executions", "error", err)
			}

			for _, run := range runs {
				s.launch(ctx, run)
			}

			schedules, err := s.Store.DueAfter(ctx, time.Now(), after)
			if err != nil && ctx.Err() == nil {
				slog.Error("read due schedules", "error", err)
			}

			// Advance even when this page is unavailable or busy. Wrap after the
			// final page so skipped occurrences are retried without changing them.
			if err == nil {
				after = nil
				if len(schedules) > 0 {
					after = &schedules[len(schedules)-1]
				}
			}

			for _, item := range schedules {
				if !s.available(ctx, item.ServerID) {
					continue
				}

				s.mu.Lock()
				if s.active[item.ServerID] || len(s.active) >= 4 {
					s.mu.Unlock()
					continue
				}

				s.active[item.ServerID] = true
				s.mu.Unlock()

				run, err := s.Store.ClaimSchedule(ctx, item.ID, time.Now(), nil)
				if err != nil {
					s.mu.Lock()
					delete(s.active, item.ServerID)
					s.mu.Unlock()

					if !errors.Is(err, repository.ErrAccessConflict) && !errors.Is(err, repository.ErrAccessNotFound) && ctx.Err() == nil {
						slog.Error("claim scheduled execution", "error", err)
					}

					continue
				}

				s.run(ctx, run)
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

// available leaves definitions pending while the assigned Spark cannot accept work.
func (s *ActivityService) available(ctx context.Context, serverID string) bool {
	server, err := s.deployment.Servers.Get(ctx, serverID)
	if err != nil {
		return false
	}

	node, err := s.deployment.Sparks.Get(ctx, server.SparkID)

	return err == nil && !node.Disabled && (s.deployment.Sparks.connectivity == nil || node.Connected)
}

func (s *ActivityService) launch(ctx context.Context, run database.ScheduleRun) {
	s.mu.Lock()
	if s.active[run.ServerID] || len(s.active) >= 4 {
		s.mu.Unlock()
		return
	}

	s.active[run.ServerID] = true
	s.mu.Unlock()
	s.run(ctx, run)
}

func (s *ActivityService) run(ctx context.Context, run database.ScheduleRun) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() { s.mu.Lock(); delete(s.active, run.ServerID); s.mu.Unlock() }()

		if err := s.process(ctx, run); err != nil && ctx.Err() == nil {
			slog.Error("process execution", "run", run.ID, "error", err)
		}
	}()
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

func (s *ActivityService) authorize(ctx context.Context, principal Principal, serverID, required string) error {
	permissions, err := s.auth.Access().ServerPermissions(ctx, principal, serverID)
	if err != nil {
		return err
	}

	if !slices.Contains(permissions, accessperm.ServerView) {
		return ErrServerNotFound
	}

	if required == "" || !slices.Contains(permissions, required) {
		return repository.ErrAccessDenied
	}

	return nil
}

// Save validates a recurrence and assigns its execution identity to the editing user.
func (s *ActivityService) Save(ctx context.Context, principal Principal, item database.Schedule, create bool) (database.Schedule, error) {
	if err := s.authorize(ctx, principal, item.ServerID, accessperm.ServerSchedulesManage); err != nil {
		return database.Schedule{}, err
	}

	if _, err := s.deployment.Servers.Get(ctx, item.ServerID); err != nil {
		return database.Schedule{}, err
	}

	if create {
		item.ID = rand.Text()
	} else {
		old, err := s.Store.Schedule(ctx, item.ID)
		if err != nil {
			return database.Schedule{}, err
		}

		if old.ServerID != item.ServerID {
			return database.Schedule{}, repository.ErrAccessInput
		}

		item.CreatedAt = old.CreatedAt
		if item.Action == "command" && old.Action == "command" && item.Command == "" {
			item.Command = old.Command
		}
	}

	item.Name = strings.TrimSpace(item.Name)
	if item.Name == "" || utf8.RuneCountInString(item.Name) > 100 || strings.ContainsFunc(item.Name, unicode.IsControl) {
		return database.Schedule{}, repository.ErrAccessInput
	}

	item.CronExpression = strings.Join(strings.Fields(item.CronExpression), " ")

	item.TimeZone = strings.TrimSpace(item.TimeZone)
	if item.TimeZone == "" {
		item.TimeZone = "UTC"
	}

	if item.CronExpression == "" {
		if item.NextRunAt <= 0 || item.Enabled && item.NextRunAt <= time.Now().Unix() {
			return database.Schedule{}, repository.ErrAccessInput
		}
	} else {
		next, err := recurrence.Next(item.CronExpression, item.TimeZone, time.Now(), 1)
		if err != nil {
			return database.Schedule{}, repository.ErrAccessInput
		}

		item.NextRunAt = next[0]
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

	item.RunAsUserID, item.Username = principal.UserID, principal.Username
	if err := s.authorize(ctx, principal, item.ServerID, accessperm.Action(item.Action)); err != nil {
		return database.Schedule{}, err
	}

	item.AuthorizationScopes = principal.TokenScopes
	if err := s.Store.SaveSchedule(ctx, &item, create); err != nil {
		return database.Schedule{}, err
	}

	return item, nil
}

// SetEnabled changes a schedule's enabled state with the same authority as editing it.
func (s *ActivityService) SetEnabled(ctx context.Context, principal Principal, id string, enabled bool) (database.Schedule, error) {
	item, err := s.Store.Schedule(ctx, id)
	if err != nil {
		return database.Schedule{}, err
	}

	if err := s.authorize(ctx, principal, item.ServerID, accessperm.ServerSchedulesManage); err != nil {
		return database.Schedule{}, err
	}

	if err := s.authorize(ctx, principal, item.ServerID, accessperm.Action(item.Action)); err != nil {
		return database.Schedule{}, err
	}

	return s.Store.SetScheduleEnabled(ctx, id, item.ServerID, enabled, principal.UserID, principal.TokenScopes)
}

// Remove stops future recurrences while retaining execution and audit history.
func (s *ActivityService) Remove(ctx context.Context, principal Principal, id string) error {
	item, err := s.Store.Schedule(ctx, id)
	if err != nil {
		return err
	}

	if err := s.authorize(ctx, principal, item.ServerID, accessperm.ServerSchedulesManage); err != nil {
		return err
	}

	if err := s.authorize(ctx, principal, item.ServerID, accessperm.Action(item.Action)); err != nil {
		return err
	}

	return s.Store.DeleteSchedule(ctx, id)
}

// RunNow starts an immediate attempt using the current saved definition.
func (s *ActivityService) RunNow(ctx context.Context, principal Principal, id string) (database.ScheduleRun, error) {
	item, err := s.Store.Schedule(ctx, id)
	if err != nil {
		return database.ScheduleRun{}, err
	}

	if err := s.authorize(ctx, principal, item.ServerID, accessperm.ServerSchedulesRun); err != nil {
		return database.ScheduleRun{}, err
	}

	if err := s.authorize(ctx, principal, item.ServerID, accessperm.Action(item.Action)); err != nil {
		return database.ScheduleRun{}, err
	}

	run, err := s.Store.ClaimSchedule(ctx, id, time.Now(), &principal.UserID, principal.TokenScopes)
	if err != nil {
		return database.ScheduleRun{}, err
	}

	if err := s.process(ctx, run); err != nil {
		return database.ScheduleRun{}, err
	}

	return s.Store.Run(ctx, run.ID)
}

// attempt records and dispatches an explicitly requested retry.
func (s *ActivityService) attempt(ctx context.Context, principal Principal, old database.ScheduleRun) (database.ScheduleRun, error) {
	server, err := s.deployment.Servers.Get(ctx, old.ServerID)
	if err != nil {
		return database.ScheduleRun{}, err
	}

	now := time.Now().Unix()

	run := database.ScheduleRun{ID: rand.Text(), ScheduleID: old.ScheduleID, RetryOf: old.ID, ServerID: server.ID, ServerName: server.Name, Action: old.Action, UserID: principal.UserID, Username: principal.Username, State: "dispatching", CreatedAt: now, StartedAt: now}

	run.AuthorizationScopes = principal.TokenScopes
	if old.ScheduleID != "" {
		run.AuthorizationPermission = accessperm.ServerSchedulesRun
	}

	if err := s.Store.CreateRun(ctx, &run); err != nil {
		return database.ScheduleRun{}, err
	}

	if err := s.process(ctx, run); err != nil {
		return database.ScheduleRun{}, err
	}

	return s.Store.Run(ctx, run.ID)
}

// Retry creates a new tracked execution and never silently replays uncertain node work.
func (s *ActivityService) Retry(ctx context.Context, principal Principal, id string) (database.ScheduleRun, error) {
	old, err := s.Store.Run(ctx, id)
	if err != nil {
		return database.ScheduleRun{}, err
	}

	if err := s.authorize(ctx, principal, old.ServerID, accessperm.Action(old.Action)); err != nil {
		return database.ScheduleRun{}, err
	}

	if old.ScheduleID != "" {
		if err := s.authorize(ctx, principal, old.ServerID, accessperm.ServerSchedulesRun); err != nil {
			return database.ScheduleRun{}, err
		}
	}

	if !slices.Contains([]string{"failed", "interrupted"}, old.State) || !slices.Contains([]string{"install", "start", "stop", "restart", "kill", "backup"}, old.Action) {
		return database.ScheduleRun{}, repository.ErrAccessInput
	}

	return s.attempt(ctx, principal, old)
}

func (s *ActivityService) finish(ctx context.Context, run database.ScheduleRun, state, message string) error {
	return s.Store.FinishRun(ctx, run, state, message, time.Now())
}

func (s *ActivityService) checkExecution(ctx context.Context, run database.ScheduleRun) (Principal, error) {
	principal, err := s.principal(ctx, run.UserID)

	principal.ScopeLimit = run.AuthorizationScopes
	if err == nil {
		err = s.authorize(ctx, principal, run.ServerID, accessperm.Action(run.Action))
		if err == nil && run.ScheduleID != "" {
			required := run.AuthorizationPermission
			if required == "" {
				required = accessperm.ServerSchedulesManage
			}

			err = s.authorize(ctx, principal, run.ServerID, required)
		}
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

func (s *ActivityService) rejectAttempt(ctx context.Context, run database.ScheduleRun, err error) error {
	if errors.Is(err, errServerNotRunning) {
		return s.finish(ctx, run, "skipped", "Server is not running; this schedule only runs while the server is running.")
	}

	if errors.Is(err, errSparkOffline) {
		return s.finish(ctx, run, "failed", "Spark became unavailable before dispatch.")
	}

	if errors.Is(err, errSparkDisabled) {
		return s.finish(ctx, run, "failed", "Hosting Spark is disabled; execution was skipped.")
	}

	if !errors.Is(err, repository.ErrAccessDenied) && !errors.Is(err, repository.ErrAccessNotFound) && !errors.Is(err, ErrServerNotFound) {
		return err
	}

	return s.finish(ctx, run, "failed", "Execution permission was revoked or the server was removed.")
}

func (s *ActivityService) process(ctx context.Context, run database.ScheduleRun) error {
	if err := s.Store.TouchRun(ctx, run.ID); err != nil {
		return err
	}

	if run.State == "dispatching" {
		principal, err := s.checkExecution(ctx, run)
		if err != nil {
			return s.rejectAttempt(ctx, run, err)
		}

		status, err := s.deployment.executeTracked(WithActor(ctx, principal), run)
		if errors.Is(err, ErrServerActive) {
			return s.finish(ctx, run, "failed", "Server already has an active operation; this execution was skipped.")
		}

		if err != nil {
			if errors.Is(err, errServerNotRunning) || errors.Is(err, errSparkOffline) || errors.Is(err, errSparkDisabled) || errors.Is(err, repository.ErrAccessDenied) || errors.Is(err, repository.ErrAccessNotFound) || errors.Is(err, ErrServerNotFound) {
				return s.rejectAttempt(ctx, run, err)
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

	if !status.Busy && status.ErrorCode == model.RuntimeOperationInterrupted {
		return s.finish(ctx, run, "interrupted", "Node operation was interrupted.")
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
	run.AuthorizationScopes = principal.TokenScopes

	return run, s.Store.CreateRun(ctx, &run)
}
