package api

import (
	"context"
	"crypto/rand"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	accessperm "campfire/internal/campfire/permissions"
	"campfire/internal/campfire/recurrence"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
)

type auditKey struct{}

func auditDetail(r *http.Request, action model.AuditAction, target string) {
	if event, ok := r.Context().Value(auditKey{}).(*database.AuditEvent); ok {
		if action != "" {
			event.Action = action
		}

		if target != "" {
			event.Target = target
		}
	}
}

func auditActor(r *http.Request, principal service.Principal) {
	if event, ok := r.Context().Value(auditKey{}).(*database.AuditEvent); ok {
		if principal.UserID != 0 {
			event.UserID = &principal.UserID
		}

		event.Username = principal.Username
		event.APITokenID = principal.APITokenID
	}
}

type auditWriter struct {
	http.ResponseWriter
	status     int
	writeError error
}

func (w *auditWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
		w.ResponseWriter.WriteHeader(status)
	}
}
func (w *auditWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}

	n, err := w.ResponseWriter.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}

	if err != nil {
		w.writeError = err
	}

	return n, err
}
func (w *auditWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (a *API) auditRequests(mux *http.ServeMux, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, pattern := mux.Handler(r)
		policy := auditPolicy(pattern)

		policy.download = policy.download && r.Method == http.MethodGet
		if a.activity == nil || (r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions) && !policy.download && pattern != "GET /api/v1/auth/oidc/callback" {
			next.ServeHTTP(w, r)
			return
		}

		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		target := "api"

		if len(parts) >= 3 {
			for _, kind := range []string{"servers", "sparks", "embers", "users", "roles", "schedules", "auth", "executions", "tokens", "backups"} {
				if parts[2] == kind {
					target = strings.TrimSuffix(kind, "s")
				}
			}
		}

		if len(parts) >= 4 && target != "auth" && target != "api" && len(parts[3]) <= 128 && strings.IndexFunc(parts[3], func(r rune) bool {
			return !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-", r)
		}) == -1 {
			target += ":" + parts[3]
		}

		action := strings.ToLower(r.Method) + "." + strings.Split(target, ":")[0]
		if target == "auth" && len(parts) >= 4 {
			for _, name := range []string{"login", "setup", "logout", "oidc"} {
				if parts[3] == name {
					action = "auth." + name
				}
			}
		}

		if policy.action != "" {
			action = string(policy.action)
		}

		target = auditRouteTarget(r, pattern, target)
		event := database.AuditEvent{ID: rand.Text(), Action: model.AuditAction(action), Target: target, Outcome: model.AuditPending, CreatedAt: a.now().Unix()}

		state := &auditRequest{event: &event, activity: a.activity, quiet: policy.quiet || policy.files}
		if !state.quiet {
			if err := state.begin(r); err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "audit storage unavailable; action was not performed"})
				return
			}
		}

		recorder := &auditWriter{ResponseWriter: w}
		complete := false

		defer func() {
			event.Status = recorder.status
			if event.Status == 0 {
				event.Status = http.StatusOK
			}

			if !complete {
				event.Outcome = model.AuditInterrupted
				event.Status = http.StatusInternalServerError
			} else if event.Outcome == model.AuditPending {
				if recorder.writeError != nil || policy.download && event.Status < 400 && (!state.downloadComplete || r.Context().Err() != nil) {
					event.Outcome = model.AuditInterrupted
				} else if event.Status >= 400 {
					event.Outcome = model.AuditFailed
				} else if event.Status == http.StatusAccepted {
					event.Outcome = model.AuditAccepted
				} else {
					event.Outcome = model.AuditSucceeded
				}
			}

			if state.quiet && event.Outcome == model.AuditSucceeded {
				return
			}

			if !state.started {
				if err := state.begin(r); err != nil {
					slog.Error("record failed audit request", "id", event.ID, "error", err)
					return
				}
			}

			ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 3*time.Second)
			defer cancel()

			if err := a.activity.Store.FinishAudit(ctx, event); err != nil {
				slog.Error("finish audit event", "id", event.ID, "error", err)
			}
		}()

		ctx := context.WithValue(r.Context(), auditKey{}, &event)
		ctx = context.WithValue(ctx, auditRequestKey{}, state)
		next.ServeHTTP(recorder, r.WithContext(ctx))

		complete = true
	})
}

func (a *API) listSchedules(w http.ResponseWriter, r *http.Request) {
	p := currentUser(r)

	items, err := a.activity.Store.Schedules(r.Context(), p.UserID, p.Admin, r.URL.Query().Get("server_id"))
	if err != nil {
		writeError(w, err)
		return
	}

	visible := make([]database.Schedule, 0, len(items))
	for _, item := range items {
		permissions, err := a.access.ServerPermissions(r.Context(), p, item.ServerID)
		if err == nil && slices.Contains(permissions, accessperm.ServerSchedulesRead) {
			visible = append(visible, item)
		}
	}

	writeJSON(w, http.StatusOK, visible)
}

func (a *API) previewSchedule(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Cron     string `json:"cron_expression"`
		TimeZone string `json:"time_zone"`
	}
	if !decodeRequest(w, r, &input, 1024) {
		return
	}

	next, err := recurrence.Next(input.Cron, input.TimeZone, a.now(), 3)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"next_runs": next})
}

func (a *API) saveSchedule(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ServerID         string             `json:"server_id"`
		Name             string             `json:"name"`
		Action           model.ServerAction `json:"action"`
		Command          string             `json:"command"`
		Enabled          bool               `json:"enabled"`
		OnlyWhenRunning  bool               `json:"only_when_running"`
		BackupMode       model.BackupMode   `json:"backup_mode,omitempty"`
		BackupStopServer bool               `json:"backup_stop_server"`
		Cron             string             `json:"cron_expression"`
		NextRunAt        int64              `json:"next_run_at"`
		TimeZone         string             `json:"time_zone"`
	}
	if !decodeRequest(w, r, &input, 8192) {
		return
	}

	id := r.PathValue("scheduleID")

	item, err := a.activity.Save(r.Context(), currentUser(r), database.Schedule{BackupStopServer: input.BackupStopServer, BackupMode: input.BackupMode, ID: id, ServerID: input.ServerID, Name: input.Name, Action: input.Action, Command: input.Command, CronExpression: input.Cron, NextRunAt: input.NextRunAt, TimeZone: input.TimeZone, OnlyWhenRunning: input.OnlyWhenRunning, Enabled: input.Enabled}, id == "")
	if err != nil {
		writeError(w, err)
		return
	}

	auditDetail(r, "schedule.save", "schedule:"+item.ID)

	status := http.StatusOK
	if id == "" {
		status = http.StatusCreated
	}

	writeJSON(w, status, item)
}

func (a *API) deleteSchedule(w http.ResponseWriter, r *http.Request) {
	if err := a.activity.Remove(r.Context(), currentUser(r), r.PathValue("scheduleID")); err != nil {
		writeError(w, err)
		return
	}

	auditDetail(r, "schedule.delete", "schedule:"+r.PathValue("scheduleID"))
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) setScheduleEnabled(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Enabled *bool `json:"enabled"`
	}
	if !decodeRequest(w, r, &input, 1024) {
		return
	}

	if input.Enabled == nil {
		writeError(w, repository.ErrAccessInput)
		return
	}

	item, err := a.activity.SetEnabled(r.Context(), currentUser(r), r.PathValue("scheduleID"), *input.Enabled)
	if err != nil {
		writeError(w, err)
		return
	}

	auditDetail(r, "schedule.toggle", "schedule:"+item.ID)
	writeJSON(w, http.StatusOK, item)
}

func (a *API) runSchedule(w http.ResponseWriter, r *http.Request) {
	run, err := a.activity.RunNow(r.Context(), currentUser(r), r.PathValue("scheduleID"))
	if err != nil {
		writeError(w, err)
		return
	}

	auditDetail(r, "schedule.run", "server:"+run.ServerID)
	writeJSON(w, http.StatusAccepted, run)
}

func (a *API) listExecutions(w http.ResponseWriter, r *http.Request) {
	p := currentUser(r)

	request, ok := pageRequest(w, r)
	if !ok {
		return
	}

	all, err := a.access.CanReadAllScheduleExecutions(r.Context(), p)
	if err != nil {
		writeError(w, err)
		return
	}

	scope := repository.ExecutionAccess{AllServers: all}
	if !scope.AllServers {
		servers, err := a.servers.ListVisible(r.Context(), p)
		if err != nil {
			writeError(w, err)
			return
		}

		if err := a.access.ApplyServerPermissions(r.Context(), p, servers); err != nil {
			writeError(w, err)
			return
		}

		for _, server := range servers {
			if slices.Contains(server.Permissions, accessperm.ServerSchedulesRead) {
				scope.ServerIDs = append(scope.ServerIDs, server.ID)
			}
		}
	}

	items, err := a.activity.Store.ExecutionPage(r.Context(), scope, r.URL.Query().Get("server_id"), r.URL.Query().Get("schedule_id"), request)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, items)
}

func (a *API) retryExecution(w http.ResponseWriter, r *http.Request) {
	run, err := a.activity.Retry(r.Context(), currentUser(r), r.PathValue("runID"))
	if err != nil {
		writeError(w, err)
		return
	}

	auditDetail(r, "schedule.retry", "server:"+run.ServerID)
	writeJSON(w, http.StatusAccepted, run)
}

func (a *API) listAudit(w http.ResponseWriter, r *http.Request) {
	request, ok := pageRequest(w, r)
	if !ok {
		return
	}

	items, err := a.activity.Store.AuditPage(r.Context(), r.URL.Query().Get("action"), r.URL.Query().Get("target"), request)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, items)
}

func (a *API) activityEnabled(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.activity == nil {
			writeError(w, repository.ErrAccessNotFound)
			return
		}

		next(w, r)
	}
}
