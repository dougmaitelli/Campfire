//go:build e2e && !production

package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"path"
	"sort"
	"strings"

	"campfire/internal/shared/model"
)

// Console uses the real SSE endpoint and delivers new output after commands.
func (r *runtime) Console(ctx context.Context, _ string, id string, emit func(model.ConsoleEvent) error) error {
	offset := 0
	ready := false
	for {
		r.mu.Lock()
		logs, updates := r.statuses[id].Logs, r.updates
		r.mu.Unlock()
		if len(logs) > offset {
			if err := emit(model.ConsoleEvent{Content: logs[offset:]}); err != nil {
				return err
			}
			offset = len(logs)
		}
		if !ready {
			if err := emit(model.ConsoleEvent{Ready: true}); err != nil {
				return err
			}
			ready = true
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-updates:
		}
	}
}

func revision(contents string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(contents))) }

// Files keeps fixture edits in memory and enforces the same revision contract as Spark.
func (r *runtime) Files(_ context.Context, _ string, input model.FileRequest) (model.FileResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := model.FileResult{ServerID: input.ServerID, Path: input.Path}
	files := r.files[input.ServerID]
	if r.fileModes == nil {
		r.fileModes = make(map[string]map[string]uint32)
	}

	if r.fileModes[input.ServerID] == nil {
		r.fileModes[input.ServerID] = make(map[string]uint32)
	}

	modes := r.fileModes[input.ServerID]
	name := path.Clean(input.Path)
	switch input.Operation {
	case "backup.list":
		result.Entries = r.backups[input.ServerID]
	case "backup.read":
		data, exists := r.archives[input.ServerID+"/"+input.Path]
		if !exists || input.Offset < 0 || input.Offset > int64(len(data)) {
			return result, &model.FileError{Code: "file_not_found", Message: "Backup not found."}
		}
		result.Size = int64(len(data))
		result.Data = data[input.Offset:min(int64(len(data)), input.Offset+model.FileLimit)]
	case "list":
		prefix := ""
		if name != "." {
			prefix = name + "/"
		}
		result.Entries = []model.FileEntry{}
		for filename, contents := range files {
			if !strings.HasPrefix(filename, prefix) {
				continue
			}
			child := strings.TrimPrefix(filename, prefix)
			directory := strings.HasSuffix(child, "/")
			child = strings.TrimSuffix(child, "/")
			if child == "" || strings.Contains(child, "/") {
				continue
			}
			kind := "file"
			if directory {
				kind = "directory"
			}
			mode, ok := modes[filename]
			if !ok {
				mode = 0644
				if directory {
					mode = 0755
				}
			}

			result.Entries = append(result.Entries, model.FileEntry{Name: child, Kind: kind, Mode: mode, Size: int64(len(contents)), ModifiedAt: fixed})
		}
		sort.Slice(result.Entries, func(i, j int) bool { return result.Entries[i].Name < result.Entries[j].Name })
	case "chmod":
		if _, directory := files[name+"/"]; directory {
			name += "/"
		}

		if _, exists := files[name]; !exists || input.Mode > 0777 {
			return result, &model.FileError{Code: "invalid_file", Message: "Invalid permissions or file."}
		}

		modes[name] = input.Mode
	case "read":
		contents, exists := files[name]
		if !exists {
			return result, &model.FileError{Code: "file_not_found", Message: "File not found."}
		}
		result.Data, result.Revision, result.Size = []byte(contents), revision(contents), int64(len(contents))
	case "write":
		contents, exists := files[name]
		if !exists || input.Revision != revision(contents) {
			return result, &model.FileError{Code: "file_conflict", Message: "The file changed; reopen it before saving."}
		}
		files[name] = string(input.Data)
		result.Revision = revision(files[name])
	default:
		return result, &model.FileError{Code: "invalid_file", Message: "This operation is not provided by the browser-test fixture."}
	}
	return result, nil
}
