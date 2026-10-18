package api

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"time"

	"campfire/internal/campfire/service"
	"campfire/internal/shared/protocol"
)

func (a *API) createSpark(w http.ResponseWriter, r *http.Request) {
	var input service.CreateSparkInput
	if !decodeRequest(w, r, &input, 16<<10) {
		return
	}

	spark, err := a.sparks.Create(r.Context(), input)
	if err != nil {
		writeError(w, err)
		return
	}

	w.Header().Set("Location", "/api/v1/sparks/"+spark.ID)
	auditDetail(r, "spark.create", "spark:"+spark.ID)
	writeJSON(w, http.StatusCreated, spark)
}

func (a *API) sparkUpdateStatus(w http.ResponseWriter, _ *http.Request) {
	if a.sparks.Updates == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Spark update checks are unavailable"})
		return
	}

	writeJSON(w, http.StatusOK, a.sparks.Updates.Status())
}

func (a *API) updateSparkClient(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Version string `json:"version"`
	}
	if !decodeRequest(w, r, &input, 1024) {
		return
	}

	if a.sparks.Updates == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Spark updates are unavailable"})
		return
	}

	// This operation outlives the ordinary panel write timeout while downloading.
	if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(protocol.SparkUpdateTimeout + 10*time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		writeError(w, err)
		return
	}

	if err := a.sparks.Updates.Update(r.Context(), r.PathValue("id"), input.Version); err != nil {
		writeError(w, err)
		return
	}

	auditDetail(r, "spark.update", "spark:"+r.PathValue("id"))
	writeJSON(w, http.StatusAccepted, map[string]string{"version": input.Version})
}

func (a *API) updateSpark(w http.ResponseWriter, r *http.Request) {
	var input service.UpdateSparkInput
	if !decodeRequest(w, r, &input, 16<<10) {
		return
	}

	spark, err := a.sparks.Update(r.Context(), r.PathValue("id"), input)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, spark)
}

func (a *API) deleteSpark(w http.ResponseWriter, r *http.Request) {
	if err := a.sparks.Delete(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}

	auditDetail(r, "spark.delete", "spark:"+r.PathValue("id"))
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) createSparkEnrollment(w http.ResponseWriter, r *http.Request) {
	if a.agents == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "the panel agent HTTPS listener is not configured"})
		return
	}

	var input struct{}
	if !decodeRequest(w, r, &input, 1024) {
		return
	}

	token, err := a.agents.CreateEnrollment(r.Context(), r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "enrollment requires an enabled Spark"})
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, token)
}

func decodeRequest(w http.ResponseWriter, r *http.Request, input any, maxBytes int64) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "Content-Type must be application/json"})
		return false
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	err = decoder.Decode(input)
	if err == nil {
		var extra any
		if trailing := decoder.Decode(&extra); !errors.Is(trailing, io.EOF) {
			err = trailing
			if err == nil {
				err = errors.New("request must contain one JSON object")
			}
		}
	}

	if err != nil {
		var tooLarge *http.MaxBytesError

		status := http.StatusBadRequest
		if errors.As(err, &tooLarge) {
			status = http.StatusRequestEntityTooLarge
		}

		writeJSON(w, status, map[string]string{"error": "invalid JSON request"})

		return false
	}

	return true
}

func (a *API) listSparks(w http.ResponseWriter, r *http.Request) {
	sparks, err := a.sparks.List(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, sparks)
}

func (a *API) getSpark(w http.ResponseWriter, r *http.Request) {
	spark, err := a.sparks.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, spark)
}
