package api

import (
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
	"errors"
	"net/http"
	"slices"
)

func (a *API) serverFiles(w http.ResponseWriter, r *http.Request) {
	if a.deployment == nil {
		writeError(w, service.ErrDeploymentUnavailable)
		return
	}

	input := model.FileRequest{Operation: "list", Path: r.URL.Query().Get("path")}
	if input.Path == "" {
		input.Path = "."
	}

	if r.Method == http.MethodPost && !decodeRequest(w, r, &input, 2<<20) {
		return
	}

	if r.Method == http.MethodPost {
		if !slices.Contains([]string{"list", "read", "write", "mkdir", "rename", "delete"}, input.Operation) {
			writeFileError(w, &model.FileError{Code: "invalid_file", Message: "Invalid game file operation."})
			return
		}

		for _, operation := range []string{"list", "read", "write", "mkdir", "rename", "delete"} {
			if input.Operation == operation {
				auditDetail(r, "server.files."+operation, "server:"+r.PathValue("id"))
			}
		}
	}

	result, err := a.deployment.Files(r.Context(), r.PathValue("id"), input)
	if err != nil {
		writeFileError(w, err)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, result)
}

func writeFileError(w http.ResponseWriter, err error) {
	var failure *model.FileError
	if !errors.As(err, &failure) {
		writeError(w, err)
		return
	}

	status := http.StatusBadGateway

	switch failure.Code {
	case "invalid_file":
		status = http.StatusBadRequest
	case "file_not_found":
		status = http.StatusNotFound
	case "file_conflict", "file_busy", "unsupported_capability":
		status = http.StatusConflict
	case "file_limit":
		status = http.StatusRequestEntityTooLarge
	}

	writeJSON(w, status, map[string]string{"error": failure.Message})
}
