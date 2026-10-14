package api

import "net/http"

// Handler builds the versioned Spark routes.
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/sparks", a.createSpark)
	mux.HandleFunc("GET /api/v1/sparks", a.listSparks)
	mux.HandleFunc("GET /api/v1/sparks/{id}", a.getSpark)
	mux.HandleFunc("PATCH /api/v1/sparks/{id}", a.updateSpark)

	return mux
}
