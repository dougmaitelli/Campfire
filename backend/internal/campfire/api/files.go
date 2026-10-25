package api

import (
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"path"
	"slices"
	"time"
)

func (a *API) fileLimits(w http.ResponseWriter, _ *http.Request) {
	if a.deployment == nil {
		writeError(w, service.ErrDeploymentUnavailable)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]int64{"max_upload_bytes": a.deployment.UploadLimit()})
}

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
		if input.Operation == "compress" || input.Operation == "extract" {
			_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(30 * time.Minute))
		}

		if !slices.Contains([]string{"list", "read", "write", "mkdir", "rename", "delete", "chmod", "stat", "read.chunk", "upload.begin", "upload.chunk", "upload.commit", "upload.cancel", "compress", "extract"}, input.Operation) {
			writeFileError(w, &model.FileError{Code: "invalid_file", Message: "Invalid game file operation."})
			return
		}

		for _, operation := range []string{"list", "read", "write", "mkdir", "rename", "delete", "chmod", "upload.begin", "upload.commit", "upload.cancel", "compress", "extract"} {
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

func (a *API) downloadFile(w http.ResponseWriter, r *http.Request) {
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(45 * time.Minute))
	if a.deployment == nil {
		writeError(w, service.ErrDeploymentUnavailable)
		return
	}

	request := model.FileRequest{Operation: "stat", Path: r.URL.Query().Get("path")}

	info, err := a.deployment.Files(r.Context(), r.PathValue("id"), request)
	if err != nil {
		writeFileError(w, err)
		return
	}

	if len(info.Entries) != 1 || info.Entries[0].Kind != "file" || info.Size < 0 || info.Size > model.FileTransferLimit {
		writeFileError(w, &model.FileError{Code: "file_limit", Message: "Select a file of at most 100 GiB."})
		return
	}

	request.Operation, request.Revision = "read.chunk", info.Revision
	// Fetch before sending headers so the first Spark error remains actionable.
	result, err := a.deployment.Files(r.Context(), r.PathValue("id"), request)
	if err != nil {
		writeFileError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": path.Base(request.Path)}))
	w.Header().Set("Content-Length", fmt.Sprint(info.Size))
	w.Header().Set("Cache-Control", "no-store")

	for request.Offset < info.Size {
		principal, err := a.requestPrincipal(r)
		if err != nil {
			panic(http.ErrAbortHandler)
		}

		permissions, err := a.access.ServerPermissions(r.Context(), principal, r.PathValue("id"))
		if err != nil || !slices.Contains(permissions, "server.view") || !slices.Contains(permissions, "server.operate") {
			panic(http.ErrAbortHandler)
		}

		if result.Revision != info.Revision || result.Size != info.Size || len(result.Data) == 0 || int64(len(result.Data)) > info.Size-request.Offset {
			panic(http.ErrAbortHandler)
		}

		if _, err := w.Write(result.Data); err != nil {
			return
		}

		request.Offset += int64(len(result.Data))
		if request.Offset < info.Size {
			result, err = a.deployment.Files(r.Context(), r.PathValue("id"), request)
			if err != nil {
				panic(http.ErrAbortHandler)
			}
		}
	}
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
