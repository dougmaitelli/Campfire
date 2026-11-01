package api

import (
	"net/http"
	"slices"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
	accessperm "campfire/internal/shared/permissions"
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

type createServerInput struct {
	model.ServerConfiguration
	OwnerUserID int64  `json:"owner_user_id"`
	BackupToken string `json:"backup_token,omitempty"`
}

func (a *API) createServer(w http.ResponseWriter, r *http.Request) {
	var input createServerInput
	if !decodeRequest(w, r, &input, 1<<20) {
		return
	}

	var (
		server model.Server
		err    error
	)
	if input.BackupToken != "" {
		server, err = a.createFromBackup(r, input)
	} else if err = a.checkHiddenVariables(r, input.ServerConfiguration, ""); err == nil {
		server, err = a.createOwnedServer(r, input)
	}

	if err != nil {
		writeError(w, err)
		return
	}

	w.Header().Set("Location", "/api/v1/servers/"+server.ID)

	server.Permissions, err = a.access.ServerPermissions(r.Context(), currentUser(r), server.ID)
	if err != nil {
		writeError(w, err)
		return
	}

	auditDetail(r, "server.create", "server:"+server.ID)
	a.writeServer(w, r, http.StatusCreated, server)
}

func (a *API) createOwnedServer(r *http.Request, input createServerInput) (model.Server, error) {
	principal := currentUser(r)

	ownerID := principal.UserID
	if input.OwnerUserID != 0 {
		if !principal.Admin {
			return model.Server{}, repository.ErrAccessDenied
		}

		ownerID = input.OwnerUserID
	}

	if _, err := a.access.Owner(r.Context(), ownerID); err != nil {
		return model.Server{}, err
	}

	allowed, err := a.access.CanProvision(r.Context(), principal, input.SparkID)
	if err != nil {
		return model.Server{}, err
	}

	if !allowed {
		return model.Server{}, repository.ErrAccessDenied
	}

	return a.servers.CreateOwned(r.Context(), input.ServerConfiguration, ownerID)
}

func (a *API) updateServer(w http.ResponseWriter, r *http.Request) {
	var input model.ServerConfiguration
	if !decodeRequest(w, r, &input, 1<<20) {
		return
	}

	if err := a.checkHiddenVariables(r, input, r.PathValue("id")); err != nil {
		writeError(w, err)
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

	a.writeServer(w, r, http.StatusOK, server)
}

func (a *API) applyServerEmber(w http.ResponseWriter, r *http.Request) {
	var input struct {
		model.ServerConfiguration
		ExpectedEmberID string `json:"expected_ember_id"`
	}
	if !decodeRequest(w, r, &input, 1<<20) {
		return
	}

	if err := a.checkHiddenVariables(r, input.ServerConfiguration, ""); err != nil {
		writeError(w, err)
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
	a.writeServer(w, r, http.StatusOK, server)
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
	definitions := make(map[string]model.Ember)

	for _, server := range servers {
		server.Permissions, err = a.access.ServerPermissions(r.Context(), currentUser(r), server.ID)
		if err != nil {
			writeError(w, err)
			return
		}

		if slices.Contains(server.Permissions, accessperm.ServerView) {
			server.CanManageAccess, err = a.access.CanManageServer(r.Context(), currentUser(r), server.ID)
			if err != nil {
				writeError(w, err)
				return
			}

			ember, loaded := definitions[server.EmberID]
			if !loaded {
				ember, err = a.embers.Get(r.Context(), server.EmberID)
				if err != nil {
					writeError(w, err)
					return
				}

				definitions[server.EmberID] = ember
			}

			visible = append(visible, visibleServer(server, ember))
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

	a.writeServer(w, r, http.StatusOK, server)
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
		Action     model.ServerAction  `json:"action"`
		Backup     string              `json:"backup,omitempty"`
		Mode       model.ReinstallMode `json:"mode,omitempty"`
		BackupMode model.BackupMode    `json:"backup_mode,omitempty"`
	}
	if !decodeRequest(w, r, &input, 1024) {
		return
	}

	switch input.Action {
	case model.ServerActionInstall, model.ServerActionUpdate, model.ServerActionReinstall, model.ServerActionStart, model.ServerActionStop, model.ServerActionRestart, model.ServerActionKill, model.ServerActionBackup, model.ServerActionRestore:
	default:
		writeError(w, service.ErrInvalidServer)
		return
	}

	if input.Mode != "" && (input.Action != model.ServerActionReinstall || (input.Mode != model.ReinstallKeep && input.Mode != model.ReinstallClean)) {
		writeError(w, service.ErrInvalidServer)
		return
	}

	if !a.requireServer(w, r, accessperm.Action(input.Action)) {
		return
	}

	if a.deployment == nil {
		writeError(w, service.ErrDeploymentUnavailable)
		return
	}

	var status model.RuntimeStatus

	if input.BackupMode != "" && input.Action != model.ServerActionBackup {
		writeError(w, service.ErrInvalidServer)
		return
	}

	auditDetail(r, model.ResourceAuditAction("server", string(input.Action)), "server:"+r.PathValue("id"))

	var err error

	switch input.Action {
	case model.ServerActionBackup:
		status, err = a.deployment.Backup(r.Context(), r.PathValue("id"), input.BackupMode)
	case model.ServerActionRestore:
		status, err = a.deployment.Restore(r.Context(), r.PathValue("id"), input.Backup)
	case model.ServerActionReinstall:
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

	status, err := a.deployment.Action(r.Context(), r.PathValue("id"), model.ServerActionStatus)
	if err != nil {
		writeError(w, err)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, status)
}
