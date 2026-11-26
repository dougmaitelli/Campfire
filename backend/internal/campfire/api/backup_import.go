package api

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"campfire/internal/campfire/database/repository"
	accessperm "campfire/internal/campfire/permissions"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/limits"
	"campfire/internal/shared/model"
)

type backupImportPreview struct {
	Token         string                    `json:"token"`
	BackupID      string                    `json:"backup_id,omitempty"`
	Mode          model.BackupMode          `json:"mode"`
	Configuration model.ServerConfiguration `json:"configuration"`
	Ember         model.Ember               `json:"ember"`
	ImportEmber   bool                      `json:"import_ember"`
}

// Source access is checked again when using a prepared receipt, not just when
// its preview was opened. The receipt is never an alternate authorization grant.
func (a *API) authorizeImportSource(r *http.Request, upload *service.BackupUpload) error {
	if !upload.FromLibrary {
		return nil
	} // Owner-bound bytes supplied by this uploader.

	return a.authorizeArchiveSource(r, upload.BackupID)
}

func (a *API) authorizeArchiveSource(r *http.Request, id string) error {
	if id == "" {
		return nil
	}

	principal, err := a.requestPrincipal(r)
	if err != nil {
		return err
	}

	_, actions, err := a.archivePermissions(r.Context(), principal, id)
	if err != nil {
		return err
	}

	if !slices.Contains(actions, accessperm.ServerBackupsDownload) {
		return repository.ErrAccessDenied
	}

	return nil
}

func (a *API) prepareArchive(w http.ResponseWriter, r *http.Request) {
	if !currentUser(r).Allows(accessperm.ServersCreate) {
		writeError(w, repository.ErrAccessDenied)
		return
	}

	if !backupImportDeadline(w) {
		return
	}

	id := r.PathValue("backupID")

	row, err := a.deployment.Backups.Store.ByID(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), service.BackupImportTimeout)
	defer cancel()

	token, err := a.backupImports.Prepare(ctx, currentUser(r).UserID, row, func(ctx context.Context, offset int64) (model.FileResult, error) {
		if err := a.authorizeArchiveSource(r.WithContext(ctx), id); err != nil {
			return model.FileResult{}, err
		}

		return a.deployment.Backups.ReadArchive(ctx, id, offset)
	})
	if err != nil {
		writeFileError(w, err)
		return
	}

	if !a.writeBackupPreview(w, r, token) {
		a.backupImports.Cancel(currentUser(r).UserID, token)
	}
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
		ServerID  string `json:"server_id"`
		Size      int64  `json:"size"`
		Name      string `json:"name"`
		StoreOnly bool   `json:"store_only"`
	}
	if !decodeRequest(w, r, &input, 1024) {
		return
	}

	if input.StoreOnly && input.ServerID != "" {
		writeError(w, repository.ErrAccessInput)
		return
	}

	if err := a.authorizeBackupUpload(r, input.ServerID); err != nil {
		writeError(w, err)
		return
	}

	input.Name = strings.TrimSpace(input.Name)
	if !utf8.ValidString(input.Name) || utf8.RuneCountInString(input.Name) > limits.ResourceNameLength || strings.ContainsFunc(input.Name, unicode.IsControl) {
		writeError(w, repository.ErrAccessInput)
		return
	}

	token, err := a.backupImports.Begin(currentUser(r).UserID, input.ServerID, input.Size, a.deployment.UploadLimit())
	if err != nil {
		writeFileError(w, err)
		return
	}

	_ = a.backupImports.With(currentUser(r).UserID, token, func(upload *service.BackupUpload) error {
		upload.OriginalName = input.Name
		upload.LibraryOnly = input.StoreOnly

		return nil
	})
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
	a.writeBackupPreview(w, r, r.PathValue("token"))
}

func (a *API) writeBackupPreview(w http.ResponseWriter, r *http.Request, token string) bool {
	if !backupImportDeadline(w) {
		return false
	}

	var preview backupImportPreview

	ctx, cancel := context.WithTimeout(r.Context(), service.BackupImportTimeout)
	defer cancel()

	err := a.backupImports.With(currentUser(r).UserID, token, func(upload *service.BackupUpload) error {
		if err := a.authorizeBackupUpload(r, upload.Target); err != nil {
			return err
		}

		if err := a.authorizeImportSource(r.WithContext(ctx), upload); err != nil {
			return err
		}

		if err := upload.Inspect(ctx); err != nil {
			return err
		}

		// Panel/library uploads are durable here. Existing-server uploads remain
		// temporary until published into that server's Spark backup store.
		if upload.Target == "" {
			if _, err := upload.Store(ctx, a.deployment.Backups); err != nil {
				return err
			}
		}

		definition := upload.Manifest.Definition

		ember, missing, err := a.backupImportEmber(ctx, definition.Ember)
		if err != nil {
			return err
		}

		if upload.Target == "" && !upload.LibraryOnly && missing && !currentUser(r).Allows(accessperm.EmbersManage) {
			return &model.FileError{Code: model.FileInvalid, Message: "An administrator must import this backup’s Ember before you can create a server from it."}
		}

		if upload.Target == "" && !upload.LibraryOnly && !currentUser(r).Allows(accessperm.EmbersManage) && backupHasHiddenOverrides(ember, definition.Configuration) {
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

		preview = backupImportPreview{Token: token, BackupID: upload.BackupID, Mode: upload.Manifest.Mode, Configuration: configuration, Ember: editorEmbers([]model.Ember{ember})[0], ImportEmber: missing}

		return nil
	})
	if err != nil {
		writeFileError(w, err)
		return false
	}

	writeJSON(w, http.StatusOK, preview)

	return true
}

func (a *API) publishBackupImport(w http.ResponseWriter, r *http.Request) {
	if !backupImportDeadline(w) {
		return
	}

	id, token := r.PathValue("id"), r.PathValue("token")

	ctx, cancel := context.WithTimeout(r.Context(), service.BackupImportTimeout)
	defer cancel()

	var name string

	err := a.backupImports.With(currentUser(r).UserID, token, func(upload *service.BackupUpload) error {
		authorize := func() error {
			principal, err := a.requestPrincipal(r.WithContext(ctx))
			if err != nil {
				return err
			}

			if err := a.authorizeImportSource(r.WithContext(ctx), upload); err != nil {
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

	auditDetail(r, "backup.import", "server:"+id)
	writeJSON(w, http.StatusCreated, map[string]string{"name": name})
}

func backupImportDeadline(w http.ResponseWriter) bool {
	deadline := time.Now().Add(service.BackupImportTimeout + 10*time.Second)
	if err := http.NewResponseController(w).SetWriteDeadline(deadline); err != nil && !errors.Is(err, http.ErrNotSupported) {
		writeError(w, err)
		return false
	}

	return true
}

func (a *API) createFromBackup(r *http.Request, input createServerInput) (model.Server, error) {
	var server model.Server

	err := a.backupImports.With(currentUser(r).UserID, input.BackupToken, func(upload *service.BackupUpload) error {
		if err := a.authorizeImportSource(r, upload); err != nil {
			return err
		}

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
