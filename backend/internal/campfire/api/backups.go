package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

	if a.deployment.Backups != nil {
		items, err := a.deployment.Backups.List(r.Context(), r.PathValue("id"))
		if err != nil {
			writeFileError(w, err)
			return
		}

		policy, err := a.deployment.Backups.Store.Policy(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, err)
			return
		}

		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, map[string]any{"server_id": r.PathValue("id"), "entries": items, "policy": policy, "s3_available": a.deployment.Backups.Remote != nil})

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
	read := func() (model.FileResult, error) {
		if a.deployment.Backups != nil {
			return a.deployment.Backups.Read(ctx, r.PathValue("id"), request.Path, request.Offset)
		}

		return a.deployment.Files(ctx, r.PathValue("id"), request)
	}

	result, err := read()
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
	digest := result.Revision
	hash := sha256.New()

	for {
		// A long download must not outlive account or server-access revocation.
		principal, err := a.requestPrincipal(r.WithContext(ctx))
		if err != nil {
			return
		}

		permissions, err := a.access.ServerPermissions(ctx, principal, r.PathValue("id"))
		if err != nil || !slices.Contains(permissions, "server.operate") {
			return
		}

		if result.Size != size || len(result.Data) == 0 || int64(len(result.Data)) > size-request.Offset {
			// An incomplete Content-Length makes interrupted downloads fail explicitly.
			return
		}

		_, _ = hash.Write(result.Data)
		if request.Offset+int64(len(result.Data)) == size && digest != "" && hex.EncodeToString(hash.Sum(nil)) != digest {
			return
		}

		if _, err := w.Write(result.Data); err != nil {
			return
		}

		request.Offset += int64(len(result.Data))
		if request.Offset == size {
			return
		}

		result, err = read()
		if err != nil {
			return
		}
	}
}

func (a *API) saveBackupPolicy(w http.ResponseWriter, r *http.Request) {
	if a.deployment == nil || a.deployment.Backups == nil {
		writeError(w, service.ErrDeploymentUnavailable)
		return
	}

	var input model.BackupPolicy
	if !decodeRequest(w, r, &input, 4096) {
		return
	}

	if err := a.deployment.Backups.SavePolicy(r.Context(), r.PathValue("id"), input); err != nil {
		writeFileError(w, err)
		return
	}

	auditDetail(r, "server.backup.policy", "server:"+r.PathValue("id"))
	writeJSON(w, http.StatusOK, input)
}

func (a *API) deleteBackup(w http.ResponseWriter, r *http.Request) {
	if a.deployment == nil || a.deployment.Backups == nil {
		writeError(w, service.ErrDeploymentUnavailable)
		return
	}

	location := r.URL.Query().Get("storage")
	if location == "" {
		location = "all"
	}

	if err := a.deployment.Backups.Delete(r.Context(), r.PathValue("id"), r.PathValue("name"), location); err != nil {
		writeFileError(w, err)
		return
	}

	auditDetail(r, "server.backup.delete", "server:"+r.PathValue("id")+" backup:"+r.PathValue("name")+" storage:"+location)
	writeJSON(w, http.StatusAccepted, map[string]string{"state": "deleting"})
}
