package api

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"slices"
	"time"

	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
)

func (a *API) serverBackups(w http.ResponseWriter, r *http.Request) {
	if a.deployment == nil {
		writeError(w, service.ErrDeploymentUnavailable)
		return
	}

	result, err := a.deployment.Files(r.Context(), r.PathValue("id"), model.FileRequest{Operation: "backup.list", Path: "."})
	if err != nil {
		writeFileError(w, err)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, result)
}

func (a *API) downloadBackup(w http.ResponseWriter, r *http.Request) {
	if a.deployment == nil {
		writeError(w, service.ErrDeploymentUnavailable)
		return
	}

	// Each chunk uses the same placement and containment checks as file access.
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Minute)
	defer cancel()

	request := model.FileRequest{Operation: "backup.read", Path: r.PathValue("name")}

	result, err := a.deployment.Files(ctx, r.PathValue("id"), request)
	if err != nil {
		writeFileError(w, err)
		return
	}

	controller := http.NewResponseController(w)
	if err := controller.SetWriteDeadline(time.Now().Add(45 * time.Minute)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		writeError(w, service.ErrDeploymentUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": request.Path}))
	w.Header().Set("Content-Length", fmt.Sprint(result.Size))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	size := result.Size

	for {
		// A long download must not outlive account or server-access revocation.
		session, _, err := a.auth.Session(ctx, browserToken(r))
		if err != nil {
			return
		}

		permissions, err := a.access.ServerPermissions(ctx, service.PrincipalFromSession(session), r.PathValue("id"))
		if err != nil || !slices.Contains(permissions, "server.operate") {
			return
		}

		if result.Size != size || len(result.Data) == 0 || int64(len(result.Data)) > size-request.Offset {
			// An incomplete Content-Length makes interrupted downloads fail explicitly.
			return
		}

		if _, err := w.Write(result.Data); err != nil {
			return
		}

		request.Offset += int64(len(result.Data))
		if request.Offset == size {
			return
		}

		result, err = a.deployment.Files(ctx, r.PathValue("id"), request)
		if err != nil {
			return
		}
	}
}
