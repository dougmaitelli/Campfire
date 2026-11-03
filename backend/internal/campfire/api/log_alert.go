package api

import (
	"net/http"
	"slices"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
	accessperm "campfire/internal/shared/permissions"
)

func (a *API) logAlerts(w http.ResponseWriter, r *http.Request) {
	if a.deployment == nil || a.deployment.LogAlerts == nil {
		writeJSON(w, http.StatusOK, []model.DetectedLogAlert{})
		return
	}

	permissions, err := a.access.ServerPermissions(r.Context(), currentUser(r), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}

	items, err := a.deployment.LogAlerts.Store.List(r.Context(), r.PathValue("id"), slices.Contains(permissions, accessperm.ServerConsoleRead))
	if err != nil {
		writeError(w, err)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, items)
}

func (a *API) dismissLogAlert(w http.ResponseWriter, r *http.Request) {
	var input struct{}
	if !decodeRequest(w, r, &input, 1024) {
		return
	}

	if a.deployment == nil || a.deployment.LogAlerts == nil {
		writeError(w, repository.ErrLogAlertNotFound)
		return
	}

	if err := a.deployment.LogAlerts.Store.Dismiss(r.Context(), r.PathValue("id"), r.PathValue("alert")); err != nil {
		writeError(w, err)
		return
	}

	auditDetail(r, model.ResourceAuditAction("server", "log-alert.dismiss"), "server:"+r.PathValue("id"))
	w.WriteHeader(http.StatusNoContent)
}
