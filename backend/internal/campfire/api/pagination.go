package api

import (
	"campfire/internal/campfire/pagination"
	"net/http"
)

func pageRequest(w http.ResponseWriter, r *http.Request) (pagination.Request, bool) {
	request, err := pagination.Parse(r.URL.Query())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return request, false
	}

	return request, true
}
