package api

import (
	"net/http"
	"slices"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
	accessperm "campfire/internal/shared/permissions"
)

func (a *API) copies(w http.ResponseWriter) *service.CopyService {
	if a.deployment == nil || a.deployment.Copies == nil {
		writeError(w, service.ErrDeploymentUnavailable)
		return nil
	}

	return a.deployment.Copies
}

func (a *API) copyTemplate(w http.ResponseWriter, r *http.Request) {
	copies := a.copies(w)
	if copies == nil {
		return
	}

	server, err := a.servers.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}

	ember, err := a.embers.Get(r.Context(), server.EmberID)
	if err != nil {
		writeError(w, err)
		return
	}

	policy, err := copies.Store.Policy(r.Context(), server.ID)
	if err != nil {
		writeError(w, err)
		return
	}

	server.Permissions, err = a.access.ServerPermissions(r.Context(), currentUser(r), server.ID)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, struct {
		Server model.Server       `json:"server"`
		Ember  model.Ember        `json:"ember"`
		Policy model.BackupPolicy `json:"policy"`
	}{visibleServer(server, ember), editorEmbers([]model.Ember{ember})[0], policy})
}

type copyServerInput struct {
	model.ServerConfiguration
	OwnerUserID int64               `json:"owner_user_id"`
	CopyFiles   bool                `json:"copy_files"`
	Policy      *model.BackupPolicy `json:"policy,omitempty"`
}

func (a *API) copyServer(w http.ResponseWriter, r *http.Request) {
	var input copyServerInput
	if !decodeRequest(w, r, &input, 1<<20) {
		return
	}

	a.createCopy(w, r, input, false)
}

func (a *API) quickCopyServer(w http.ResponseWriter, r *http.Request) {
	var input struct {
		SparkID string `json:"spark_id"`
	}
	if !decodeRequest(w, r, &input, 1024) {
		return
	}

	a.createCopy(w, r, copyServerInput{ServerConfiguration: model.ServerConfiguration{SparkID: input.SparkID}, CopyFiles: true}, true)
}

func (a *API) createCopy(w http.ResponseWriter, r *http.Request, input copyServerInput, quick bool) {
	copies := a.copies(w)
	if copies == nil {
		return
	}

	if input.CopyFiles && !a.copyFilesAllowed(w, r, r.PathValue("id")) {
		return
	}

	ownerID, err := a.serverCreationOwner(r, input.OwnerUserID, input.SparkID)
	if err != nil {
		writeError(w, err)
		return
	}

	server, err := copies.Create(r.Context(), r.PathValue("id"), input.ServerConfiguration, ownerID, quick, input.CopyFiles, input.Policy)
	if err != nil {
		writeError(w, err)
		return
	}

	server.Permissions, err = a.access.ServerPermissions(r.Context(), currentUser(r), server.ID)
	if err != nil {
		writeError(w, err)
		return
	}

	w.Header().Set("Location", "/api/v1/servers/"+server.ID)
	auditDetail(r, "server.copy", "server:"+server.ID)
	a.writeServer(w, r, http.StatusCreated, server)
}

func (a *API) copyFilesAllowed(w http.ResponseWriter, r *http.Request, sourceID string) bool {
	permissions, err := a.access.ServerPermissions(r.Context(), currentUser(r), sourceID)
	if err != nil {
		writeError(w, err)
		return false
	}

	if !slices.Contains(permissions, accessperm.ServerConfigure) || !slices.Contains(permissions, accessperm.ServerFilesRead) {
		writeError(w, repository.ErrAccessDenied)
		return false
	}

	return true
}
