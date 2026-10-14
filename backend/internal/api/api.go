// Package api provides HTTP transport and thin request handlers.
package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"campfire/internal/service"
	"campfire/internal/spark"
)

// API translates HTTP requests into service calls.
type API struct {
	sparks     *service.SparkService
	embers     *service.EmberService
	servers    *service.ServerService
	agents     *spark.Hub
	deployment *service.DeploymentService
}

// WithDeployment enables explicit delivery of saved server configurations.
func (a *API) WithDeployment(deployment *service.DeploymentService) *API {
	a.deployment = deployment
	if deployment != nil {
		a.servers.WithRuntimeGuard(deployment)
	}

	return a
}

// WithAgentHub enables remote node enrollment and connection status.
func (a *API) WithAgentHub(hub *spark.Hub) *API { a.agents = hub; return a }

// New constructs the HTTP API with its application service.
func New(sparks *service.SparkService, embers *service.EmberService, servers *service.ServerService) *API {
	return &API{sparks: sparks, embers: embers, servers: servers}
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
	case errors.Is(err, service.ErrServerActive):
		status, message = http.StatusConflict, service.ErrServerActive.Error()
	case errors.Is(err, service.ErrDeploymentUnavailable):
		status, message = http.StatusServiceUnavailable, service.ErrDeploymentUnavailable.Error()
	case errors.Is(err, service.ErrInvalidSpark), errors.Is(err, service.ErrInvalidDescription), errors.Is(err, service.ErrEmptySparkUpdate), errors.Is(err, service.ErrInvalidSettings), errors.Is(err, service.ErrInvalidEmber), errors.Is(err, service.ErrInvalidServer):
		status, message = http.StatusBadRequest, err.Error()
	case errors.Is(err, service.ErrSparkConflict):
		status, message = http.StatusConflict, service.ErrSparkConflict.Error()
	case errors.Is(err, service.ErrSparkNotFound):
		status, message = http.StatusNotFound, service.ErrSparkNotFound.Error()
	case errors.Is(err, service.ErrEmberNotFound), errors.Is(err, service.ErrServerNotFound):
		status, message = http.StatusNotFound, err.Error()
	case errors.Is(err, service.ErrServerConflict), errors.Is(err, service.ErrSparkReservations):
		status, message = http.StatusConflict, err.Error()
	default:
		slog.Error("API request failed", "error", err)
	}

	writeJSON(w, status, map[string]string{"error": message})
}
