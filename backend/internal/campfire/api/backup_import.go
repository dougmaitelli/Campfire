package api

import (
	"context"
	"maps"
	"net/http"
	"time"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
	accessperm "campfire/internal/shared/permissions"
)

type backupImportPreview struct {
	Token         string                    `json:"token"`
	Mode          model.BackupMode          `json:"mode"`
	Configuration model.ServerConfiguration `json:"configuration"`
	Ember         model.Ember               `json:"ember"`
	ImportEmber   bool                      `json:"import_ember"`
}

func backupHasHiddenOverrides(ember model.Ember, configuration model.ServerConfiguration) bool {
	for _, variable := range ember.Variables {
		if value, exists := configuration.Variables[variable.Name]; variable.Hidden && exists && value != variable.Default {
			return true
		}
	}

	return false
}

func (a *API) authorizeBackupUpload(r *http.Request, target string) error {
	if a.deployment == nil {
		return service.ErrDeploymentUnavailable
	}

	if target == "" {
		if !currentUser(r).Allows(accessperm.ServersCreate) {
			return repository.ErrAccessDenied
		}

		return nil
	}

	permissions, err := a.access.ServerPermissions(r.Context(), currentUser(r), target)
	if err != nil {
		return err
	}

	for _, permission := range permissions {
		if permission == accessperm.ServerBackupsCreate {
			return nil
		}
	}

	return repository.ErrAccessDenied
}

func (a *API) beginBackupImport(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ServerID string `json:"server_id"`
		Size     int64  `json:"size"`
	}
	if !decodeRequest(w, r, &input, 1024) {
		return
	}

	if err := a.authorizeBackupUpload(r, input.ServerID); err != nil {
		writeError(w, err)
		return
	}

	token, err := a.backupImports.Begin(currentUser(r).UserID, input.ServerID, input.Size, a.deployment.UploadLimit())
	if err != nil {
		writeFileError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"token": token})
}

func (a *API) chunkBackupImport(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Offset int64  `json:"offset"`
		Data   []byte `json:"data"`
	}
	if !decodeRequest(w, r, &input, 2*model.FileLimit) {
		return
	}

	err := a.backupImports.With(currentUser(r).UserID, r.PathValue("token"), func(upload *service.BackupUpload) error {
		if err := a.authorizeBackupUpload(r, upload.Target); err != nil {
			return err
		}

		return upload.Chunk(input.Offset, input.Data)
	})
	if err != nil {
		writeFileError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, struct{}{})
}

func (a *API) cancelBackupImport(w http.ResponseWriter, r *http.Request) {
	a.backupImports.Cancel(currentUser(r).UserID, r.PathValue("token"))
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) backupImportEmber(ctx context.Context, source model.Ember) (model.Ember, bool, error) {
	embers, err := a.embers.List(ctx)
	if err != nil {
		return model.Ember{}, false, err
	}

	for _, ember := range embers {
		if model.EmberHash(ember) == model.EmberHash(source) {
			return ember, false, nil
		}
	}

	return source, true, nil
}

func (a *API) inspectBackupImport(w http.ResponseWriter, r *http.Request) {
	var preview backupImportPreview

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()

	err := a.backupImports.With(currentUser(r).UserID, r.PathValue("token"), func(upload *service.BackupUpload) error {
		if err := a.authorizeBackupUpload(r, upload.Target); err != nil {
			return err
		}

		if err := upload.Inspect(ctx); err != nil {
			return err
		}

		definition := upload.Manifest.Definition

		ember, missing, err := a.backupImportEmber(ctx, definition.Ember)
		if err != nil {
			return err
		}

		if upload.Target == "" && missing && !currentUser(r).Allows(accessperm.EmbersManage) {
			return &model.FileError{Code: model.FileInvalid, Message: "An administrator must import this backup’s Ember before you can create a server from it."}
		}

		if upload.Target == "" && !currentUser(r).Allows(accessperm.EmbersManage) && backupHasHiddenOverrides(ember, definition.Configuration) {
			return &model.FileError{Code: model.FileInvalid, Message: "An administrator must recreate this backup’s hidden variable overrides."}
		}

		configuration := definition.Configuration
		configuration.EmberID = ember.ID

		configuration.Variables = maps.Clone(configuration.Variables)
		for _, variable := range ember.Variables {
			if variable.Hidden {
				delete(configuration.Variables, variable.Name)
			}
		}

		preview = backupImportPreview{Token: r.PathValue("token"), Mode: upload.Manifest.Mode, Configuration: configuration, Ember: editorEmbers([]model.Ember{ember})[0], ImportEmber: missing}

		return nil
	})
	if err != nil {
		writeFileError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, preview)
}

func (a *API) publishBackupImport(w http.ResponseWriter, r *http.Request) {
	id, token := r.PathValue("id"), r.PathValue("token")

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()

	var name string

	err := a.backupImports.With(currentUser(r).UserID, token, func(upload *service.BackupUpload) error {
		authorize := func() error {
			principal, err := a.requestPrincipal(r.WithContext(ctx))
			if err != nil {
				return err
			}

			permissions, err := a.access.ServerPermissions(ctx, principal, id)
			if err != nil {
				return err
			}

			for _, permission := range permissions {
				if permission == accessperm.ServerBackupsCreate {
					return nil
				}
			}

			return repository.ErrAccessDenied
		}
		if err := upload.Publish(ctx, a.deployment, id, authorize); err != nil {
			return err
		}

		name = upload.Name

		return nil
	})
	if err != nil {
		writeFileError(w, err)
		return
	}

	a.backupImports.Cancel(currentUser(r).UserID, token)
	auditDetail(r, "backup.import", "server:"+id)
	writeJSON(w, http.StatusCreated, map[string]string{"name": name})
}

func (a *API) createFromBackup(r *http.Request, input createServerInput) (model.Server, error) {
	var server model.Server

	err := a.backupImports.With(currentUser(r).UserID, input.BackupToken, func(upload *service.BackupUpload) error {
		if upload.CreatedServer != "" {
			var err error

			server, err = a.servers.Get(r.Context(), upload.CreatedServer)

			return err
		}

		if upload.Target != "" || upload.Manifest.Definition == nil {
			return repository.ErrAccessDenied
		}

		definition := upload.Manifest.Definition

		ember, missing, err := a.backupImportEmber(r.Context(), definition.Ember)
		if err != nil {
			return err
		}

		if !currentUser(r).Allows(accessperm.EmbersManage) && backupHasHiddenOverrides(ember, definition.Configuration) {
			return repository.ErrAccessDenied
		}

		if missing {
			if !currentUser(r).Allows(accessperm.EmbersManage) {
				return repository.ErrAccessDenied
			}

			ember, err = a.embers.Import(r.Context(), definition.Ember)
			if err != nil {
				return err
			}
		}

		input.EmberID = ember.ID

		input.Variables = maps.Clone(input.Variables)
		if input.Variables == nil {
			input.Variables = make(map[string]string)
		}

		for _, variable := range ember.Variables {
			if variable.Hidden {
				original := definition.Configuration.Variables[variable.Name]
				if supplied, ok := input.Variables[variable.Name]; ok && supplied != original {
					return repository.ErrAccessDenied
				}

				input.Variables[variable.Name] = original
			}
		}

		server, err = a.createOwnedServer(r, input)
		if err == nil {
			upload.CreatedServer, upload.Target = server.ID, server.ID
		}

		return err
	})

	return server, err
}
