package api

import (
	"context"
	"crypto/rand"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
)

type auditKey struct{}

func auditDetail(r *http.Request, action, target string) {
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
		event.UserID, event.Username = principal.UserID, principal.Username
	}
}

type auditWriter struct {
	http.ResponseWriter
	status int
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

	return w.ResponseWriter.Write(data)
}
func (w *auditWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (a *API) auditRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.activity == nil || (r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions) && r.URL.Path != "/api/v1/auth/oidc/callback" {
			next.ServeHTTP(w, r)
			return
		}

		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		target := "api"

		if len(parts) >= 3 {
			for _, kind := range []string{"servers", "sparks", "embers", "users", "roles", "schedules", "auth", "executions"} {
				if parts[2] == kind {
					target = strings.TrimSuffix(kind, "s")
				}
			}
		}

		if len(parts) >= 4 && target != "auth" && len(parts[3]) <= 128 && strings.IndexFunc(parts[3], func(r rune) bool {
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

		event := database.AuditEvent{ID: rand.Text(), Action: action, Target: target, Outcome: "pending", CreatedAt: time.Now().Unix()}
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 3*time.Second)
		err := a.activity.Store.BeginAudit(ctx, &event)

		cancel()

		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "audit storage unavailable; action was not performed"})
			return
		}

		recorder := &auditWriter{ResponseWriter: w}
		complete := false

		defer func() {
			event.Status = recorder.status
			if event.Status == 0 {
				event.Status = http.StatusOK
			}

			if !complete {
				event.Outcome = "interrupted"
				event.Status = http.StatusInternalServerError
			} else if event.Outcome == "pending" {
				if event.Status >= 400 {
					event.Outcome = "failed"
				} else if event.Status == http.StatusAccepted {
					event.Outcome = "accepted"
				} else {
					event.Outcome = "succeeded"
				}
			}

			ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 3*time.Second)
			defer cancel()

			if err := a.activity.Store.FinishAudit(ctx, event); err != nil {
				slog.Error("finish audit event", "id", event.ID, "error", err)
			}
		}()

		next.ServeHTTP(recorder, r.WithContext(context.WithValue(r.Context(), auditKey{}, &event)))

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

	writeJSON(w, http.StatusOK, items)
}

func (a *API) saveSchedule(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ServerID string `json:"server_id"`
		Name     string `json:"name"`
		Action   string `json:"action"`
		Command  string `json:"command"`
		Interval int64  `json:"interval_minutes"`
		Next     int64  `json:"next_run_at"`
		Enabled  bool   `json:"enabled"`
	}
	if !decodeRequest(w, r, &input, 8192) {
		return
	}

	id := r.PathValue("scheduleID")

	item, err := a.activity.Save(r.Context(), currentUser(r), database.Schedule{ID: id, ServerID: input.ServerID, Name: input.Name, Action: input.Action, Command: input.Command, IntervalMinutes: input.Interval, NextRunAt: input.Next, Enabled: input.Enabled}, id == "")
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

	items, err := a.activity.Store.Runs(r.Context(), p.UserID, p.Admin, r.URL.Query().Get("server_id"), r.URL.Query().Get("before"))
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
	items, err := a.activity.Store.Audit(r.Context(), r.URL.Query().Get("before"), r.URL.Query().Get("action"), r.URL.Query().Get("target"))
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
