package api

import (
	"context"
	"net/http"
	"time"

	"campfire/internal/campfire/service"
)

func (a *API) monitoring(w http.ResponseWriter, r *http.Request) {
	if a.deployment == nil {
		writeError(w, service.ErrDeploymentUnavailable)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()

	snapshot, err := a.deployment.Monitoring(ctx, currentUser(r), a.access)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, snapshot)
}

func (a *API) serverMetrics(w http.ResponseWriter, r *http.Request) {
	if a.deployment == nil {
		writeError(w, service.ErrDeploymentUnavailable)
		return
	}

	usage, err := a.deployment.Metrics(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, usage)
}
