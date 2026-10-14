package api

import "net/http"

// Handler builds the versioned application routes.
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/sparks", a.createSpark)
	mux.HandleFunc("GET /api/v1/sparks", a.listSparks)
	mux.HandleFunc("GET /api/v1/sparks/{id}", a.getSpark)
	mux.HandleFunc("PATCH /api/v1/sparks/{id}", a.updateSpark)
	mux.HandleFunc("POST /api/v1/sparks/{id}/enrollment", a.createSparkEnrollment)
	mux.HandleFunc("GET /api/v1/embers", a.listEmbers)
	mux.HandleFunc("POST /api/v1/embers", a.importEmber)
	mux.HandleFunc("POST /api/v1/embers/preview-url", a.previewEmberURL)
	mux.HandleFunc("GET /api/v1/embers/{id}", a.getEmber)
	mux.HandleFunc("GET /api/v1/servers", a.listServers)
	mux.HandleFunc("POST /api/v1/servers", a.createServer)
	mux.HandleFunc("GET /api/v1/servers/{id}", a.getServer)
	mux.HandleFunc("PUT /api/v1/servers/{id}", a.updateServer)
	mux.HandleFunc("DELETE /api/v1/servers/{id}", a.deleteServer)
	mux.HandleFunc("POST /api/v1/servers/{id}/configuration", a.syncServerConfiguration)
	mux.HandleFunc("POST /api/v1/servers/{id}/actions", a.serverAction)
	mux.HandleFunc("GET /api/v1/servers/{id}/status", a.serverStatus)
	mux.HandleFunc("GET /api/v1/servers/{id}/console", a.serverConsole)
	mux.HandleFunc("POST /api/v1/servers/{id}/console", a.serverConsole)
	mux.HandleFunc("GET /api/v1/servers/{id}/files", a.serverFiles)
	mux.HandleFunc("POST /api/v1/servers/{id}/files", a.serverFiles)

	return mux
}
