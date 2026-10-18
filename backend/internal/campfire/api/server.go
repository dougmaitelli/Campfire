package api

import (
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
	"net/http"
	"slices"
)

func (a *API) syncServerConfiguration(w http.ResponseWriter, r *http.Request) {
	var input struct{}
	if !decodeRequest(w, r, &input, 1024) {
		return
	}

	if a.deployment == nil {
		writeError(w, service.ErrDeploymentUnavailable)
		return
	}

	receipt, err := a.deployment.Sync(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, receipt)
}

func (a *API) createServer(w http.ResponseWriter, r *http.Request) {
	var input struct {
		model.ServerConfiguration
		OwnerUserID int64 `json:"owner_user_id"`
	}
	if !decodeRequest(w, r, &input, 1<<20) {
		return
	}

	principal := currentUser(r)

	ownerID := principal.UserID
	if input.OwnerUserID != 0 {
		if !principal.Admin {
			writeError(w, repository.ErrAccessDenied)
			return
		}

		ownerID = input.OwnerUserID
	}

	if _, err := a.access.Owner(r.Context(), ownerID); err != nil {
		writeError(w, err)
		return
	}

	allowed, err := a.access.CanProvision(r.Context(), principal, input.SparkID)
	if err != nil {
		writeError(w, err)
		return
	}

	if !allowed {
		writeError(w, repository.ErrAccessDenied)
		return
	}

	server, err := a.servers.CreateOwned(r.Context(), input.ServerConfiguration, ownerID)
	if err != nil {
		writeError(w, err)
		return
	}

	w.Header().Set("Location", "/api/v1/servers/"+server.ID)

	server.Permissions, err = a.access.ServerPermissions(r.Context(), principal, server.ID)
	if err != nil {
		writeError(w, err)
		return
	}

	auditDetail(r, "server.create", "server:"+server.ID)
	writeJSON(w, http.StatusCreated, server)
}

func (a *API) updateServer(w http.ResponseWriter, r *http.Request) {
	var input model.ServerConfiguration
	if !decodeRequest(w, r, &input, 1<<20) {
		return
	}

	server, err := a.servers.Update(r.Context(), r.PathValue("id"), input)
	if err != nil {
		writeError(w, err)
		return
	}

	server.Permissions, err = a.access.ServerPermissions(r.Context(), currentUser(r), server.ID)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, server)
}

func (a *API) applyServerEmber(w http.ResponseWriter, r *http.Request) {
	var input struct {
		model.ServerConfiguration
		ExpectedEmberID string `json:"expected_ember_id"`
	}
	if !decodeRequest(w, r, &input, 1<<20) {
		return
	}

	server, err := a.servers.ApplyEmber(r.Context(), r.PathValue("id"), input.ExpectedEmberID, input.ServerConfiguration)
	if err != nil {
		writeError(w, err)
		return
	}

	server.Permissions, err = a.access.ServerPermissions(r.Context(), currentUser(r), server.ID)
	if err != nil {
		writeError(w, err)
		return
	}

	auditDetail(r, "server.ember.apply", "server:"+server.ID+" ember:"+input.ExpectedEmberID+" -> "+server.EmberID)
	writeJSON(w, http.StatusOK, server)
}

func (a *API) listServers(w http.ResponseWriter, r *http.Request) {
	servers, err := a.servers.List(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}

	if a.deployment != nil {
		for i := range servers {
			servers[i].Status = a.deployment.CachedState(servers[i].ID)
		}
	}

	visible := make([]model.Server, 0, len(servers))
	for _, server := range servers {
		server.Permissions, err = a.access.ServerPermissions(r.Context(), currentUser(r), server.ID)
		if err != nil {
			writeError(w, err)
			return
		}

		if slices.Contains(server.Permissions, "server.view") {
			visible = append(visible, server)
		}
	}

	writeJSON(w, http.StatusOK, visible)
}

func (a *API) getServer(w http.ResponseWriter, r *http.Request) {
	server, err := a.servers.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}

	if a.deployment != nil {
		server.Status = a.deployment.CachedState(server.ID)
	}

	server.Permissions, err = a.access.ServerPermissions(r.Context(), currentUser(r), server.ID)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, server)
}

func (a *API) deleteServer(w http.ResponseWriter, r *http.Request) {
	if a.deployment != nil {
		if err := a.deployment.Delete(r.Context(), r.PathValue("id")); err != nil {
			writeFileError(w, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)

		return
	}

	if err := a.servers.Delete(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *API) serverAction(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Action string `json:"action"`
		Backup string `json:"backup,omitempty"`
		Mode   string `json:"mode,omitempty"`
	}
	if !decodeRequest(w, r, &input, 1024) {
		return
	}

	switch input.Action {
	case "install", "update", "reinstall", "start", "stop", "restart", "kill", "backup", "restore":
	default:
		writeError(w, service.ErrInvalidServer)
		return
	}

	if input.Mode != "" && (input.Action != "reinstall" || (input.Mode != "keep" && input.Mode != "clean")) {
		writeError(w, service.ErrInvalidServer)
		return
	}

	if input.Action == "update" || input.Action == "reinstall" {
		permissions, err := a.access.ServerPermissions(r.Context(), currentUser(r), r.PathValue("id"))
		if err != nil {
			writeError(w, err)
			return
		}

		if !slices.Contains(permissions, "server.configure") {
			writeError(w, repository.ErrAccessDenied)
			return
		}
	}

	if a.deployment == nil {
		writeError(w, service.ErrDeploymentUnavailable)
		return
	}

	var status model.RuntimeStatus

	auditDetail(r, "server."+input.Action, "server:"+r.PathValue("id"))

	var err error

	switch input.Action {
	case "restore":
		status, err = a.deployment.Restore(r.Context(), r.PathValue("id"), input.Backup)
	case "reinstall":
		status, err = a.deployment.Reinstall(r.Context(), r.PathValue("id"), input.Mode)
	default:
		status, err = a.deployment.Action(r.Context(), r.PathValue("id"), input.Action)
	}

	if err != nil {
		writeFileError(w, err)
		return
	}

	writeJSON(w, http.StatusAccepted, status)
}

func (a *API) serverConsole(w http.ResponseWriter, r *http.Request) {
	if a.deployment == nil {
		writeError(w, service.ErrDeploymentUnavailable)
		return
	}

	var input struct {
		Command string `json:"command"`
	}
	if r.Method == http.MethodPost && !decodeRequest(w, r, &input, 8192) {
		return
	}

	status, err := a.deployment.Console(r.Context(), r.PathValue("id"), input.Command, r.Method == http.MethodGet)
	if err != nil {
		writeError(w, err)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, status)
}

func (a *API) serverStatus(w http.ResponseWriter, r *http.Request) {
	if a.deployment == nil {
		writeError(w, service.ErrDeploymentUnavailable)
		return
	}

	status, err := a.deployment.Action(r.Context(), r.PathValue("id"), "status")
	if err != nil {
		writeError(w, err)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, status)
}
