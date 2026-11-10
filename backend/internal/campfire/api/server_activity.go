package api

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	integrationservice "campfire/internal/campfire/service/integrations"
	accessperm "campfire/internal/shared/permissions"
)

func (a *API) serverActivityAccess(r *http.Request) (repository.ServerActivityAccess, error) {
	grants, err := a.access.ServerPermissions(r.Context(), currentUser(r), r.PathValue("id"))

	access := repository.ServerActivityAccess{
		Console: slices.Contains(grants, accessperm.ServerConsoleRead), Backups: slices.Contains(grants, accessperm.ServerBackupsRead),
		ManualOperations: slices.Contains(grants, accessperm.ServerOperate), ScheduledOperations: slices.Contains(grants, accessperm.ServerSchedulesRead),
		JobTypes: jobHistoryTypes(currentUser(r), grants), ManageIntegrations: slices.Contains(grants, accessperm.ServerConfigure),
		ManageNotifications: currentUser(r).Allows(accessperm.NotificationsManage) && a.notifications != nil && a.notifications.Apprise != nil,
	}
	for _, source := range integrationservice.NotificationSources() {
		if slices.Contains(grants, source.Permission) {
			access.Categories = append(access.Categories, source.ID)
		}
	}

	return access, err
}

func (a *API) activityStore(w http.ResponseWriter) *repository.ServerActivityRepository {
	if a.serverActivity == nil {
		writeError(w, service.ErrDeploymentUnavailable)
	}

	return a.serverActivity
}

func (a *API) serverActivityList(w http.ResponseWriter, r *http.Request) {
	store := a.activityStore(w)
	if store == nil {
		return
	}

	access, err := a.serverActivityAccess(r)
	if err != nil {
		writeError(w, err)
		return
	}

	query := r.URL.Query()

	filter := repository.ServerActivityFilter{Kind: query.Get("kind"), Search: query.Get("search"), State: query.Get("state"), Before: query.Get("before"), GameType: query.Get("type"), Fields: map[string]string{}}
	for _, field := range []struct {
		name  string
		value *int64
	}{{"from", &filter.From}, {"to", &filter.To}} {
		if raw := query.Get(field.name); raw != "" {
			value, parseErr := strconv.ParseInt(raw, 10, 64)
			if parseErr != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid activity time range."})
				return
			}

			*field.value = value
		}
	}

	for name, values := range query {
		if strings.HasPrefix(name, "data.") {
			if len(values) != 1 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Use each field filter once."})
				return
			}

			filter.Fields[strings.TrimPrefix(name, "data.")] = values[0]
		}
	}

	if err := filter.Validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	page, err := store.List(r.Context(), r.PathValue("id"), access, filter)
	if err != nil {
		if errors.Is(err, repository.ErrActivityInput) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid activity cursor."})
			return
		}

		writeError(w, err)

		return
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, page)
}

func (a *API) serverActivityFilters(w http.ResponseWriter, r *http.Request) {
	store := a.activityStore(w)
	if store == nil {
		return
	}

	choices, err := store.FilterChoices(r.Context(), r.PathValue("id"), r.URL.Query().Get("kind"))
	if err != nil {
		if errors.Is(err, repository.ErrActivityInput) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid activity type."})
			return
		}

		writeError(w, err)

		return
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, choices)
}

func (a *API) serverActivityDetails(w http.ResponseWriter, r *http.Request) {
	store := a.activityStore(w)
	if store == nil {
		return
	}

	access, err := a.serverActivityAccess(r)
	if err != nil {
		writeError(w, err)
		return
	}

	details, err := store.Details(r.Context(), r.PathValue("id"), r.PathValue("entryKey"), access)
	if err != nil {
		writeError(w, err)
		return
	}

	var job *jobProgress

	if details.Job != nil {
		value, err := a.publicJob(r, *details.Job)
		if err != nil {
			writeError(w, err)
			return
		}

		job = &value
	}

	canRetry := false

	if run := details.Operation; run != nil && a.activity != nil && service.RetryableExecution(*run) {
		grants, err := a.access.ServerPermissions(r.Context(), currentUser(r), r.PathValue("id"))
		if err != nil {
			writeError(w, err)
			return
		}

		canRetry = slices.Contains(grants, accessperm.Action(run.Action)) && (run.ScheduleID == "" || slices.Contains(grants, accessperm.ServerSchedulesRun))
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, struct {
		repository.ServerActivityDetails
		Job               *jobProgress `json:"job,omitempty"`
		CanRetryOperation bool         `json:"can_retry_operation"`
	}{details, job, canRetry})
}

func (a *API) serverActivityNotifications(w http.ResponseWriter, r *http.Request) {
	store := a.activityStore(w)
	if store == nil {
		return
	}

	access, err := a.serverActivityAccess(r)
	if err != nil {
		writeError(w, err)
		return
	}

	page, err := store.Notifications(r.Context(), r.PathValue("id"), r.PathValue("entryKey"), access, r.URL.Query().Get("before"))
	if err != nil {
		if errors.Is(err, repository.ErrActivityInput) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid activity cursor."})
			return
		}

		writeError(w, err)

		return
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, page)
}

func (a *API) retryActivityNotification(w http.ResponseWriter, r *http.Request) {
	store := a.activityStore(w)
	if store == nil {
		return
	}

	var input struct{}
	if !decodeRequest(w, r, &input, 1024) {
		return
	}

	access, err := a.serverActivityAccess(r)
	if err != nil {
		writeError(w, err)
		return
	}

	details, err := store.Details(r.Context(), r.PathValue("id"), r.PathValue("entryKey"), access)
	if err != nil {
		writeError(w, err)
		return
	}

	item := details.Notification
	if item == nil || !item.CanRetry {
		writeError(w, repository.ErrAccessDenied)
		return
	}

	if item.BindingID != "" {
		if a.integrations == nil {
			writeError(w, service.ErrDeploymentUnavailable)
			return
		}

		grants, err := a.access.ServerPermissions(r.Context(), currentUser(r), r.PathValue("id"))
		if err != nil {
			writeError(w, err)
			return
		}

		err = a.integrations.RetryServerMessage(r.Context(), r.PathValue("id"), item.BindingID, grants, item.MessageID)
		if err != nil {
			writeError(w, err)
			return
		}
	} else if err := store.RetryPanelDelivery(r.Context(), r.PathValue("id"), item.MessageID, time.Now()); err != nil {
		writeError(w, err)
		return
	}

	auditDetail(r, "notification.retry", "server:"+r.PathValue("id"))
	w.WriteHeader(http.StatusNoContent)
}
