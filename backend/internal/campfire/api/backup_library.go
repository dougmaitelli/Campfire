package api

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
)

func (a *API) archivePermissions(ctx context.Context, principal service.Principal, id string) (database.Backup, []model.PermissionID, error) {
	if a.deployment == nil || a.deployment.Backups == nil {
		return database.Backup{}, nil, service.ErrDeploymentUnavailable
	}

	row, err := a.deployment.Backups.Store.ByID(ctx, id)
	if err != nil {
		return row, nil, err
	}

	var actions []model.PermissionID
	if row.ServerID != "" {
		actions, err = a.access.ServerPermissions(ctx, principal, row.ServerID)
	} else {
		actions, err = a.access.IndependentBackupPermissions(ctx, principal, row.OwnerUserID)
	}

	return row, actions, err
}

type authorizedArchiveKey struct{}

func (a *API) archiveAccess(action model.PermissionID, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		archive, actions, err := a.archivePermissions(r.Context(), currentUser(r), r.PathValue("backupID"))
		if err != nil {
			writeError(w, err)
			return
		}

		if !slices.Contains(actions, action) {
			writeError(w, repository.ErrAccessDenied)
			return
		}

		next(w, r.WithContext(context.WithValue(r.Context(), authorizedArchiveKey{}, archive)))
	}
}

func backupEditTarget(serverID string, backup model.Backup, changes model.BackupChanges) string {
	target := "backup:" + backup.ID
	if serverID != "" {
		target = "server:" + serverID + " backup:" + backup.Name
	}

	actions := make([]string, 0, 2)
	if changes.DisplayName != nil {
		actions = append(actions, "rename")
	}

	if changes.Pinned != nil {
		if *changes.Pinned {
			actions = append(actions, "pin")
		} else {
			actions = append(actions, "unpin")
		}
	}

	return target + " changes:" + strings.Join(actions, ",")
}

func (a *API) updateArchive(w http.ResponseWriter, r *http.Request) {
	var input model.BackupChanges
	if !decodeRequest(w, r, &input, 4096) {
		return
	}

	archive, ok := r.Context().Value(authorizedArchiveKey{}).(database.Backup)
	if !ok {
		writeError(w, repository.ErrAccessDenied)
		return
	}

	item, err := a.deployment.Backups.UpdateArchive(r.Context(), archive, input)
	if err != nil {
		writeFileError(w, err)
		return
	}

	auditDetail(r, "server.backup.update", backupEditTarget(archive.ServerID, item, input))
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, item)
}

func (a *API) deleteArchive(w http.ResponseWriter, r *http.Request) {
	archive, ok := r.Context().Value(authorizedArchiveKey{}).(database.Backup)
	if !ok {
		writeError(w, repository.ErrAccessDenied)
		return
	}

	if err := a.deployment.Backups.DeleteArchive(r.Context(), archive); err != nil {
		writeFileError(w, err)
		return
	}

	auditDetail(r, "server.backup.delete", "backup:"+r.PathValue("backupID"))
	writeJSON(w, http.StatusAccepted, map[string]string{"state": "deleting"})
}

func (a *API) downloadArchive(w http.ResponseWriter, r *http.Request) { a.streamBackup(w, r, true) }
