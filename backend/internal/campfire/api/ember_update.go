package api

import (
	"fmt"
	"net/http"

	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
)

func (a *API) listEmberUpdates(w http.ResponseWriter, r *http.Request) {
	items := []model.EmberUpdate{}

	if a.embers.Updates != nil {
		var err error

		items, err = a.embers.Updates.Store.List(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, items)
}

func (a *API) checkEmberUpdate(w http.ResponseWriter, r *http.Request) {
	a.fetchEmberUpdate(w, r, false)
}

func (a *API) downloadEmberUpdate(w http.ResponseWriter, r *http.Request) {
	a.fetchEmberUpdate(w, r, true)
}

func (a *API) fetchEmberUpdate(w http.ResponseWriter, r *http.Request, download bool) {
	var input struct{}
	if !decodeRequest(w, r, &input, 1024) {
		return
	}

	if a.embers.Updates == nil {
		writeError(w, service.ErrDeploymentUnavailable)
		return
	}

	item, preview, err := a.embers.Updates.Check(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}

	auditDetail(r, "ember.check-update", "ember:"+r.PathValue("id"))

	if download {
		if item.Error != "" {
			writeError(w, fmt.Errorf("%w: %s", service.ErrInvalidEmber, item.Error))
			return
		}

		writeJSON(w, http.StatusOK, preview)

		return
	}

	writeJSON(w, http.StatusOK, item)
}
