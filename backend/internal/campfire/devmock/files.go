//go:build !production

package devmock

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"path"
	"sort"
	"strings"
	"time"

	"campfire/internal/shared/model"
)

func fileError(code, message string) error {
	return &model.FileError{Code: code, Message: message}
}

func revision(data string) string {
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

func mockFileContent(data string) string {
	if encoded, ok := strings.CutPrefix(data, "\x00archive:"); ok {
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err == nil {
			return string(decoded)
		}
	}

	return data
}

func cleanPath(value string) (string, error) {
	value = path.Clean(value)
	if path.IsAbs(value) || value == ".." || strings.HasPrefix(value, "../") || strings.ContainsAny(value, "\x00\\") {
		return "", fileError("invalid_file", "Use a relative game file path.")
	}

	return value, nil
}

// Files keeps small editable game files in SQLite, never on the host filesystem.
// Directory keys end in a slash; game-file keys do not.
func (r *Runtime) Files(ctx context.Context, spark string, request model.FileRequest) (model.FileResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	record, err := r.load(ctx, spark, request.ServerID)
	if err != nil {
		return model.FileResult{}, err
	}

	if strings.HasPrefix(request.Operation, "backup.") {
		return r.backupFiles(ctx, record, request)
	}

	name, err := cleanPath(request.Path)
	if err != nil {
		return model.FileResult{}, err
	}

	result := model.FileResult{ServerID: request.ServerID, Path: name}
	data, exists := record.Files[name]

	directory := name == "."
	if _, ok := record.Files[name+"/"]; ok {
		directory = true
	}

	switch request.Operation {
	case "list":
		if !directory {
			return result, fileError("file_not_found", "Directory not found.")
		}

		result.Entries = []model.FileEntry{}

		for key, value := range record.Files {
			if path.Dir(strings.TrimSuffix(key, "/")) != name {
				continue
			}

			entry := model.FileEntry{Name: path.Base(key), Kind: "file", Size: int64(len(mockFileContent(value))), ModifiedAt: time.Unix(1700000000, 0).UTC()}
			if strings.HasSuffix(key, "/") {
				entry.Kind = "directory"
			}

			result.Entries = append(result.Entries, entry)
		}

		sort.Slice(result.Entries, func(i, j int) bool { return result.Entries[i].Name < result.Entries[j].Name })

		return result, nil
	case "read", "stat", "read.chunk":
		data = mockFileContent(data)

		if !exists {
			return result, fileError("file_not_found", "File not found.")
		}

		result.Revision, result.Size = revision(data), int64(len(data))
		if request.Revision != "" && request.Revision != result.Revision {
			return result, fileError("file_conflict", "File changed. Reload before reading.")
		}

		switch request.Operation {
		case "read":
			result.Data = []byte(data)
		case "read.chunk":
			if request.Offset < 0 || request.Offset > int64(len(data)) {
				return result, fileError("invalid_file", "Invalid file offset.")
			}

			result.Data = []byte(data[request.Offset:min(int64(len(data)), request.Offset+model.FileLimit)])
		}

		return result, nil
	case "write":
		expected := "absent"
		if exists {
			expected = revision(data)
		}

		if directory || request.Revision != expected {
			return result, fileError("file_conflict", "File changed or already exists. Reload before saving.")
		}

		if len(request.Data) > model.FileLimit || len(record.Files) >= 256 && !exists {
			return result, fileError("file_limit", "Mock mode supports up to 256 files of 1 MiB each.")
		}

		if parent := path.Dir(name); parent != "." {
			if _, ok := record.Files[parent+"/"]; !ok {
				return result, fileError("file_not_found", "Parent directory not found.")
			}
		}

		record.Files[name] = string(request.Data)
		if quotaEnforced(record) && diskUsage(record) > record.Deployment.Configuration.DiskMB*(1<<20) {
			return result, fileError("file_limit", "Mock server's game-data allocation would be exceeded.")
		}

		result.Revision = revision(string(request.Data))
	case "mkdir":
		if exists || directory || len(record.Files) >= 256 {
			return result, fileError("file_conflict", "Directory already exists or mock file limit reached.")
		}

		if parent := path.Dir(name); parent != "." {
			if _, ok := record.Files[parent+"/"]; !ok {
				return result, fileError("file_not_found", "Parent directory not found.")
			}
		}

		record.Files[name+"/"] = ""
	case "delete":
		if name == "." {
			return result, fileError("invalid_file", "The server root cannot be deleted.")
		}
		if !exists && !directory {
			return result, fileError("file_not_found", "File not found.")
		}

		if directory {
			for key := range record.Files {
				if strings.HasPrefix(key, name+"/") {
					delete(record.Files, key)
				}
			}

			delete(record.Files, name+"/")
		} else {
			delete(record.Files, name)
		}
	case "rename":
		destination, err := cleanPath(request.Destination)
		if err != nil {
			return result, err
		}

		if (!exists && !directory) || name == "." {
			return result, fileError("file_not_found", "File or folder not found.")
		}

		if destination == name || strings.HasPrefix(destination, name+"/") {
			return result, fileError("invalid_file", "Choose a destination outside the source folder.")
		}

		if _, ok := record.Files[destination]; ok || destination == "." {
			return result, fileError("file_conflict", "Destination already exists.")
		}

		if _, ok := record.Files[destination+"/"]; ok {
			return result, fileError("file_conflict", "Destination already exists.")
		}

		if parent := path.Dir(destination); parent != "." {
			if _, ok := record.Files[parent+"/"]; !ok {
				return result, fileError("file_not_found", "Parent directory not found.")
			}
		}

		if directory {
			moved := make(map[string]string)

			for key, value := range record.Files {
				if strings.HasPrefix(key, name+"/") {
					moved[destination+strings.TrimPrefix(key, name)] = value
					delete(record.Files, key)
				}
			}

			for key, value := range moved {
				record.Files[key] = value
			}
		} else {
			record.Files[destination] = data
			delete(record.Files, name)
		}
	case "compress":
		if err := mockCompress(&record, request); err != nil {
			return result, err
		}
	default:
		return result, fileError("unsupported_capability", "This file operation is not simulated in mock mode. Use the text editor for small files.")
	}

	return result, r.db.WithContext(ctx).Save(&record).Error
}
