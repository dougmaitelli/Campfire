package api

import (
	"net/http"
	"slices"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
	accessperm "campfire/internal/shared/permissions"
)

func (a *API) platformPermission(permission model.PermissionID, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !currentUser(r).Allows(permission) {
			writeError(w, repository.ErrAccessDenied)
			return
		}

		next(w, r)
	}
}

func (a *API) requireServer(w http.ResponseWriter, r *http.Request, action model.PermissionID) bool {
	permissions, err := a.access.ServerPermissions(r.Context(), currentUser(r), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return false
	}

	if !slices.Contains(permissions, accessperm.ServerView) {
		writeError(w, service.ErrServerNotFound)
		return false
	}

	if action == "" || !slices.Contains(permissions, action) {
		writeError(w, repository.ErrAccessDenied)
		return false
	}

	return true
}

func (a *API) serverOwner(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.requireServer(w, r, accessperm.ServerView) {
			return
		}

		allowed, err := a.access.CanManageServer(r.Context(), currentUser(r), r.PathValue("id"))
		if err != nil {
			writeError(w, err)
			return
		}

		if !allowed {
			writeError(w, repository.ErrAccessDenied)
			return
		}

		next(w, r)
	}
}

func (a *API) serverPermission(permission model.PermissionID, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		permissions, err := a.access.ServerPermissions(r.Context(), currentUser(r), r.PathValue("id"))
		if err != nil {
			writeError(w, err)
			return
		}

		if !slices.Contains(permissions, accessperm.ServerView) {
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
