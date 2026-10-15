package api

import (
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
	"net/http"
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
	var input model.ServerConfiguration
	if !decodeRequest(w, r, &input, 1<<20) {
		return
	}

	server, err := a.servers.Create(r.Context(), input)
	if err != nil {
		writeError(w, err)
		return
	}

	w.Header().Set("Location", "/api/v1/servers/"+server.ID)
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

	writeJSON(w, http.StatusOK, servers)
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

	writeJSON(w, http.StatusOK, server)
}

func (a *API) deleteServer(w http.ResponseWriter, r *http.Request) {
	if a.deployment != nil {
		if err := a.deployment.Delete(r.Context(), r.PathValue("id")); err != nil {
			writeError(w, err)
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
	}
	if !decodeRequest(w, r, &input, 1024) {
		return
	}

	switch input.Action {
	case "install", "start", "stop", "restart", "kill", "backup", "restore":
	default:
		writeError(w, service.ErrInvalidServer)
		return
	}

	if a.deployment == nil {
		writeError(w, service.ErrDeploymentUnavailable)
		return
	}

	var status model.RuntimeStatus

	var err error
	if input.Action == "restore" {
		status, err = a.deployment.Restore(r.Context(), r.PathValue("id"), input.Backup)
	} else {
		status, err = a.deployment.Action(r.Context(), r.PathValue("id"), input.Action)
	}

	if err != nil {
		writeError(w, err)
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
