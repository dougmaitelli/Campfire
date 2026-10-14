// Package api provides HTTP transport and thin request handlers.
package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"campfire/internal/service"
)

// API translates HTTP requests into service calls.
type API struct{ sparks *service.SparkService }

// New constructs the HTTP API with its application service.
func New(sparks *service.SparkService) *API {
	return &API{sparks: sparks}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Error("write API response", "error", err)
	}
}

func writeError(w http.ResponseWriter, err error) {
	status, message := http.StatusInternalServerError, "internal server error"

	switch {
	case errors.Is(err, service.ErrInvalidSpark), errors.Is(err, service.ErrInvalidDescription), errors.Is(err, service.ErrEmptySparkUpdate), errors.Is(err, service.ErrInvalidSettings):
		status, message = http.StatusBadRequest, err.Error()
	case errors.Is(err, service.ErrSparkConflict):
		status, message = http.StatusConflict, service.ErrSparkConflict.Error()
	case errors.Is(err, service.ErrSparkNotFound):
		status, message = http.StatusNotFound, service.ErrSparkNotFound.Error()
	default:
		slog.Error("API request failed", "error", err)
	}

	writeJSON(w, status, map[string]string{"error": message})
}
