package api

import (
	"context"
	"net/http"
	"strconv"
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

	snapshot, err := a.deployment.Monitoring(ctx, currentUser(r))
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, snapshot)
}

func (a *API) serverResourceHistory(w http.ResponseWriter, r *http.Request) {
	a.resourceHistory(w, r, false)
}
func (a *API) sparkResourceHistory(w http.ResponseWriter, r *http.Request) {
	a.resourceHistory(w, r, true)
}

func (a *API) resourceHistory(w http.ResponseWriter, r *http.Request, spark bool) {
	if a.deployment == nil || a.deployment.History == nil {
		writeError(w, service.ErrDeploymentUnavailable)
		return
	}

	if spark {
		if _, err := a.sparks.Get(r.Context(), r.PathValue("id")); err != nil {
			writeError(w, err)
			return
		}
	}

	values := r.URL.Query()

	var from, to int64

	var limit int

	var err error
	if values.Has("from") {
		from, err = strconv.ParseInt(values.Get("from"), 10, 64)
	}

	if err == nil && values.Has("to") {
		to, err = strconv.ParseInt(values.Get("to"), 10, 64)
	}

	if err == nil && values.Has("limit") {
		limit, err = strconv.Atoi(values.Get("limit"))
	}

	if err != nil {
		writeError(w, service.ErrResourceHistoryInput)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()

	history, err := a.deployment.History.History(ctx, spark, r.PathValue("id"), from, to, limit)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, history)
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
