package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"slices"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
	permissions "campfire/internal/shared/permissions"
)

type jobProgress struct {
	repository.JobHistory
	CanRetry         bool     `json:"can_retry"`
	CanCancel        bool     `json:"can_cancel"`
	WaitingForSparks []string `json:"waiting_for_sparks"`
}

func (a *API) jobStore(w http.ResponseWriter) *repository.JobRepository {
	if a.deployment != nil {
		if a.deployment.Copies != nil {
			return a.deployment.Copies.Store.Jobs()
		}

		if a.deployment.Transfers != nil {
			return a.deployment.Transfers.Store.Jobs()
		}
	}

	writeError(w, service.ErrDeploymentUnavailable)

	return nil
}

func (a *API) publicJob(r *http.Request, job repository.JobHistory) (jobProgress, error) {
	item := jobProgress{JobHistory: job, WaitingForSparks: []string{}}
	if a.deployment != nil {
		item.WaitingForSparks = a.deployment.JobWaitingFor(job.ServerJob)
	}

	if !job.Active() {
		return item, nil
	}

	canManage := currentUser(r).Admin && currentUser(r).Allows(permissions.AllAccess)
	if !canManage && job.Type == model.JobTransfer {
		canManage = currentUser(r).Allows(permissions.SparksManage) && currentUser(r).Allows(permissions.ServersManageAll)
	} else if !canManage {
		source, err := a.access.ServerPermissions(r.Context(), currentUser(r), job.SourceServerID)
		if err != nil && !errors.Is(err, repository.ErrAccessNotFound) && !errors.Is(err, service.ErrServerNotFound) {
			return item, err
		}

		allowed, err := a.access.CanProvision(r.Context(), currentUser(r), job.TargetSparkID)
		if err != nil {
			return item, err
		}

		canManage = currentUser(r).Allows(permissions.ServersCreate) && allowed && slices.Contains(source, permissions.ServerConfigure) && slices.Contains(source, permissions.ServerFilesRead)
		if canManage {
			target, err := a.access.ServerPermissions(r.Context(), currentUser(r), job.TargetServerID)
			if err != nil {
				return item, err
			}

			canManage = slices.Contains(target, permissions.ServerConfigure)
		}
	}

	item.CanRetry = canManage && job.Status == model.JobFailed
	item.CanCancel = canManage && job.Status != model.JobCancelling && job.Step != model.JobCleanup && (job.Type != model.JobTransfer || job.Step != model.JobCommitting)

	return item, nil
}

func (a *API) listJobs(w http.ResponseWriter, r *http.Request) {
	serverID := r.PathValue("id")
	if serverID == "" && (!currentUser(r).Admin || !currentUser(r).Allows(permissions.AllAccess)) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Admin access is required."})
		return
	}

	store := a.jobStore(w)
	if store == nil {
		return
	}

	types := make([]model.JobType, 0, 2)

	filter := repository.JobFilter{}
	if serverID == "" {
		filter.Type = model.JobType(r.URL.Query().Get("type"))

		filter.Status = model.JobStatus(r.URL.Query().Get("status"))
		if filter.Type != "" && filter.Type != model.JobCopy && filter.Type != model.JobTransfer || !slices.Contains([]model.JobStatus{"", model.JobPending, model.JobRunning, model.JobFailed, model.JobCancelling, model.JobCompleted, model.JobCancelled}, filter.Status) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid job filter"})
			return
		}
	} else {
		grants, err := a.access.ServerPermissions(r.Context(), currentUser(r), serverID)
		if err != nil {
			writeError(w, err)
			return
		}

		if slices.Contains(grants, permissions.ServerConfigure) {
			types = append(types, model.JobCopy)
		}

		if currentUser(r).Allows(permissions.SparksManage) && currentUser(r).Allows(permissions.ServersManageAll) {
			types = append(types, model.JobTransfer)
		}
	}

	var before *repository.JobCursor

	if cursor := r.URL.Query().Get("before"); cursor != "" {
		data, err := base64.RawURLEncoding.DecodeString(cursor)

		var value repository.JobCursor
		if err != nil || len(data) > 512 || json.Unmarshal(data, &value) != nil || value.ID == "" || value.CreatedAt < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid job cursor"})
			return
		}

		before = &value
	}

	var (
		jobs []repository.JobHistory
		err  error
	)

	if serverID == "" {
		jobs, err = store.ListAll(r.Context(), filter, before)
	} else {
		jobs, err = store.List(r.Context(), serverID, types, before)
	}

	if err != nil {
		writeError(w, err)
		return
	}

	next := ""

	if len(jobs) > 50 {
		jobs = jobs[:50]
		last := jobs[len(jobs)-1]
		data, _ := json.Marshal(repository.JobCursor{CreatedAt: last.CreatedAt, ID: last.ID})
		next = base64.RawURLEncoding.EncodeToString(data)
	}

	items := make([]jobProgress, 0, len(jobs))
	for _, job := range jobs {
		item, err := a.publicJob(r, job)
		if err != nil {
			writeError(w, err)
			return
		}

		items = append(items, item)
	}

	writeJSON(w, http.StatusOK, struct {
		Items []jobProgress `json:"items"`
		Next  string        `json:"next"`
	}{items, next})
}

func (a *API) retryJob(w http.ResponseWriter, r *http.Request)  { a.changeJob(w, r, false) }
func (a *API) cancelJob(w http.ResponseWriter, r *http.Request) { a.changeJob(w, r, true) }

func (a *API) changeJob(w http.ResponseWriter, r *http.Request, cancel bool) {
	store := a.jobStore(w)
	if store == nil {
		return
	}

	var input struct{}
	if !decodeRequest(w, r, &input, 1024) {
		return
	}

	job, err := store.HistoryItem(r.Context(), r.PathValue("jobID"))
	if err != nil {
		writeError(w, err)
		return
	}

	visible := false
	if job.Type == model.JobTransfer {
		visible = currentUser(r).Allows(permissions.SparksManage) && currentUser(r).Allows(permissions.ServersManageAll)
	} else {
		for _, id := range []string{job.SourceServerID, job.TargetServerID} {
			grants, err := a.access.ServerPermissions(r.Context(), currentUser(r), id)
			if err == nil && slices.Contains(grants, permissions.ServerConfigure) {
				visible = true
			}
		}
	}

	if !visible {
		writeError(w, repository.ErrAccessNotFound)
		return
	}

	item, err := a.publicJob(r, job)
	if err != nil {
		writeError(w, err)
		return
	}

	if cancel && !item.CanCancel || !cancel && !item.CanRetry {
		writeError(w, repository.ErrAccessDenied)
		return
	}

	if err := store.Change(r.Context(), job.ID, cancel); err != nil {
		writeError(w, err)
		return
	}

	job, err = store.HistoryItem(r.Context(), job.ID)
	if err != nil {
		writeError(w, err)
		return
	}

	item, err = a.publicJob(r, job)
	if err != nil {
		writeError(w, err)
		return
	}

	action := "retry"
	if cancel {
		action = "cancel"
	}

	auditDetail(r, model.ResourceAuditAction("server.job", action), "server:"+job.TargetServerID)
	writeJSON(w, http.StatusAccepted, item)
}
