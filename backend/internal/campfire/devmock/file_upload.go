//go:build !production

package devmock

import (
	"crypto/rand"
	"path"
	"time"

	"campfire/internal/shared/model"
)

// Uploads track offsets only: payload bytes are discarded and never saved.
type mockUpload struct {
	serverID string
	path     string
	size     int64
	offset   int64
	expires  time.Time
}

// Called with r.mu held, after resolving the server and validating its path.
func (r *Runtime) uploadFile(record runtimeRecord, request model.FileRequest, result model.FileResult) (model.FileResult, error) {
	for id, upload := range r.uploads {
		if time.Now().After(upload.expires) {
			delete(r.uploads, id)
		}
	}

	if request.Operation == "upload.begin" {
		if request.Size < 0 || request.Size > request.UploadLimit {
			return result, fileError("file_limit", "File exceeds the upload limit.")
		}

		_, exists := record.Files[result.Path]
		_, directory := record.Files[result.Path+"/"]

		if exists || directory || result.Path == "." || request.Revision != "absent" {
			return result, fileError("file_conflict", "Destination already exists.")
		}

		if parent := path.Dir(result.Path); parent != "." {
			if _, ok := record.Files[parent+"/"]; !ok {
				return result, fileError("file_not_found", "Parent directory not found.")
			}
		}

		if len(r.uploads) >= 64 {
			return result, fileError("file_limit", "Too many mock uploads are active.")
		}

		if r.uploads == nil {
			r.uploads = make(map[string]*mockUpload)
		}

		result.UploadID = rand.Text()
		r.uploads[result.UploadID] = &mockUpload{serverID: request.ServerID, path: result.Path, size: request.Size, expires: time.Now().Add(15 * time.Minute)}

		return result, nil
	}

	upload := r.uploads[request.UploadID]
	if upload == nil || upload.serverID != request.ServerID || upload.path != result.Path {
		return result, fileError("invalid_file", "Upload session not found.")
	}

	switch request.Operation {
	case "upload.chunk":
		if request.Offset != upload.offset || len(request.Data) > model.FileLimit || upload.offset+int64(len(request.Data)) > upload.size {
			return result, fileError("invalid_file", "Invalid upload offset or size.")
		}

		upload.offset += int64(len(request.Data))
		upload.expires = time.Now().Add(15 * time.Minute)
	case "upload.commit":
		if upload.offset != upload.size {
			return result, fileError("invalid_file", "Upload is incomplete.")
		}

		delete(r.uploads, request.UploadID)
	case "upload.cancel":
		delete(r.uploads, request.UploadID)
	default:
		return result, fileError("invalid_file", "Unknown upload operation.")
	}

	return result, nil
}
