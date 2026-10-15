package api

import (
	"net/http"
	"slices"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
)

func (a *API) platformPermission(permission string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !currentUser(r).Allows(permission) {
			writeError(w, repository.ErrAccessDenied)
			return
		}

		next(w, r)
	}
}

func (a *API) serverPermission(permission string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		permissions, err := a.access.ServerPermissions(r.Context(), currentUser(r), r.PathValue("id"))
		if err != nil {
			writeError(w, err)
			return
		}

		if !slices.Contains(permissions, "server.view") {
			writeError(w, service.ErrServerNotFound)
			return
		}

		if !slices.Contains(permissions, permission) {
			writeError(w, repository.ErrAccessDenied)
			return
		}

		next(w, r)
	}
}
