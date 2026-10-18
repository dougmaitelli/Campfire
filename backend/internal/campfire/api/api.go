// Package api provides HTTP transport and thin request handlers.
package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"campfire/internal/campfire/agents"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/protocol"
)

// API translates HTTP requests into service calls.
type API struct {
	sparks        *service.SparkService
	embers        *service.EmberService
	servers       *service.ServerService
	agents        *agents.Hub
	deployment    *service.DeploymentService
	auth          *service.AuthService
	access        *service.AccessService
	origin        string
	oidc          *service.OIDCService
	activity      *service.ActivityService
	notifications *service.NotificationService
}

// WithNotifications enables personal inboxes and monitoring notification controls.
func (a *API) WithNotifications(notifications *service.NotificationService) *API {
	a.notifications = notifications
	return a
}

// WithActivity enables schedules, retained executions and the audit boundary.
func (a *API) WithActivity(activity *service.ActivityService) *API { a.activity = activity; return a }

// WithOIDC enables the optional provider login routes.
func (a *API) WithOIDC(provider *service.OIDCService) *API { a.oidc = provider; return a }

// WithAuthentication enables the mandatory administrator boundary for all panel routes.
func (a *API) WithAuthentication(auth *service.AuthService, origin string) *API {
	a.auth = auth
	if auth != nil {
		a.access = auth.Access()
	}

	a.origin = origin

	return a
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
func (a *API) WithAgentHub(hub *agents.Hub) *API { a.agents = hub; return a }

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

	var commandError *protocol.CommandError

	switch {
	case errors.Is(err, service.ErrSparkUpdate):
		status, message = http.StatusConflict, err.Error()
	case errors.As(err, &commandError) && commandError.Code == "spark_update_failed":
		status, message = http.StatusConflict, commandError.Message
	case errors.As(err, &commandError) && commandError.Code == "unsupported_capability":
		status, message = http.StatusConflict, commandError.Message
	case errors.Is(err, repository.ErrAccessDenied):
		status, message = http.StatusForbidden, err.Error()
	case errors.Is(err, repository.ErrTransferActive), errors.Is(err, repository.ErrTransferState):
		status, message = http.StatusConflict, err.Error()
	case errors.Is(err, repository.ErrSparkInUse), errors.Is(err, repository.ErrEmberInUse):
		status, message = http.StatusConflict, err.Error()
	case errors.Is(err, repository.ErrAccessInput), errors.Is(err, service.ErrAuthInput):
		status, message = http.StatusBadRequest, err.Error()
	case errors.Is(err, repository.ErrAccessConflict), errors.Is(err, repository.ErrLastAdmin):
		status, message = http.StatusConflict, err.Error()
	case errors.Is(err, repository.ErrAccessNotFound):
		status, message = http.StatusNotFound, err.Error()
	case errors.Is(err, repository.ErrNotificationNotFound):
		status, message = http.StatusNotFound, err.Error()
	case errors.Is(err, service.ErrServerActive):
		status, message = http.StatusConflict, service.ErrServerActive.Error()
	case errors.Is(err, service.ErrDeploymentUnavailable):
		status, message = http.StatusServiceUnavailable, service.ErrDeploymentUnavailable.Error()
	case errors.Is(err, service.ErrInvalidSpark), errors.Is(err, service.ErrInvalidDescription), errors.Is(err, service.ErrEmptySparkUpdate), errors.Is(err, service.ErrInvalidSettings), errors.Is(err, service.ErrInvalidEmber), errors.Is(err, service.ErrInvalidServer):
		status, message = http.StatusBadRequest, err.Error()
	case errors.Is(err, repository.ErrServerPlacement), errors.Is(err, repository.ErrServerConflict):
		status, message = http.StatusConflict, err.Error()
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
