//go:build !production

package devmock

import (
	"bytes"
	"context"
	"encoding/base64"
	"io/fs"
	"path"
	"sort"
	"strings"

	"campfire/internal/shared/archive"
	"campfire/internal/shared/model"
)

func mockCompress(ctx context.Context, record *runtimeRecord, request model.FileRequest) error {
	destination, err := cleanPath(request.Destination)
	if err != nil {
		return fileError(model.FileInvalid, "Choose a valid archive destination.")
	}

	if _, exists := record.Files[destination]; exists {
		return fileError(model.FileConflict, "Archive destination already exists.")
	}

	if parent := path.Dir(destination); parent != "." {
		if _, exists := record.Files[parent+"/"]; !exists {
			return fileError(model.FileNotFound, "Destination folder not found.")
		}
	}

	sources := request.Paths
	if len(sources) == 0 {
		sources = []string{request.Path}
	}

	if len(sources) > 1000 {
		return fileError(model.FileLimitExceeded, "Select at most 1,000 archive items.")
	}

	selected := make(map[string]bool)

	for _, source := range sources {
		name, err := cleanPath(source)
		if err != nil || name != source || (len(request.Paths) > 0 && path.Dir(name) != request.Path) || destination == name || strings.HasPrefix(destination, name+"/") {
			return fileError(model.FileInvalid, "Choose items in the current directory and a destination outside them.")
		}

		found := false

		for key := range record.Files {
			if key == name || strings.HasPrefix(key, name+"/") || name == "." {
				selected[key] = true
				found = true
			}
		}

		if !found {
			return fileError(model.FileNotFound, "Selected item not found.")
		}
	}

	keys := make([]string, 0, len(selected))
	for key := range selected {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	var buffer bytes.Buffer

	encoder, err := archive.NewWriter(ctx, &buffer, destination)
	if err != nil {
		return err
	}

	for _, key := range keys {
		name := key
		if base := path.Dir(sources[0]); base != "." {
			name = strings.TrimPrefix(name, base+"/")
		}

		content := mockFileContent(record.Files[key])

		entry := archive.Entry{Name: name, Mode: 0600, Size: int64(len(content))}
		if strings.HasSuffix(key, "/") {
			entry.Mode, entry.Size = fs.ModeDir|0700, 0
		}

		if err := encoder.WriteHeader(entry); err != nil {
			return err
		}

		if entry.Mode.IsRegular() {
			if _, err := encoder.Write([]byte(content)); err != nil {
				return err
			}
		}
	}

	if err := encoder.Close(); err != nil {
		return err
	}

	record.Files[destination] = "\x00archive:" + base64.StdEncoding.EncodeToString(buffer.Bytes())

	return nil
}
