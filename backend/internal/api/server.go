package api

import (
	"campfire/internal/model"
	"campfire/internal/service"
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

	writeJSON(w, http.StatusOK, servers)
}

func (a *API) getServer(w http.ResponseWriter, r *http.Request) {
	server, err := a.servers.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, server)
}

func (a *API) deleteServer(w http.ResponseWriter, r *http.Request) {
	if err := a.servers.Delete(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
