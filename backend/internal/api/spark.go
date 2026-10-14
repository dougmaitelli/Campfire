package api

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	"campfire/internal/service"
)

func (a *API) createSpark(w http.ResponseWriter, r *http.Request) {
	var input service.CreateSparkInput
	if !decodeSparkRequest(w, r, &input) {
		return
	}

	spark, err := a.sparks.Create(r.Context(), input)
	if err != nil {
		writeError(w, err)
		return
	}

	w.Header().Set("Location", "/api/v1/sparks/"+spark.ID)
	writeJSON(w, http.StatusCreated, spark)
}

func (a *API) updateSpark(w http.ResponseWriter, r *http.Request) {
	var input service.UpdateSparkInput
	if !decodeSparkRequest(w, r, &input) {
		return
	}

	spark, err := a.sparks.Update(r.Context(), r.PathValue("id"), input)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, spark)
}

func decodeSparkRequest(w http.ResponseWriter, r *http.Request, input any) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "Content-Type must be application/json"})
		return false
	}

	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
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

		writeJSON(w, status, map[string]string{"error": "invalid Spark request"})

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
