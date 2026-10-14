package api

import (
	"campfire/internal/model"
	"campfire/internal/service"
	"net/http"
)

func (a *API) previewEmberURL(w http.ResponseWriter, r *http.Request) {
	var input struct {
		URL string `json:"url"`
	}
	if !decodeRequest(w, r, &input, 8192) {
		return
	}

	ember, err := service.FetchEmberURL(r.Context(), input.URL)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, ember)
}

func (a *API) importEmber(w http.ResponseWriter, r *http.Request) {
	var input model.Ember
	if !decodeRequest(w, r, &input, 1<<20) {
		return
	}

	ember, err := a.embers.Import(r.Context(), input)
	if err != nil {
		writeError(w, err)
		return
	}

	w.Header().Set("Location", "/api/v1/embers/"+ember.ID)
	writeJSON(w, http.StatusCreated, ember)
}

func (a *API) listEmbers(w http.ResponseWriter, r *http.Request) {
	embers, err := a.embers.List(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, embers)
}

func (a *API) getEmber(w http.ResponseWriter, r *http.Request) {
	ember, err := a.embers.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, ember)
}
