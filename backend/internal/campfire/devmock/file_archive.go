//go:build !production

package devmock

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"path"
	"sort"
	"strings"

	"campfire/internal/shared/model"
)

func mockCompress(record *runtimeRecord, request model.FileRequest) error {
	destination, err := cleanPath(request.Destination)
	if err != nil || !strings.HasSuffix(destination, ".tar.gz") {
		return fileError(model.FileInvalid, "Choose a .tar.gz archive destination.")
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

	zipped := gzip.NewWriter(&buffer)
	archive := tar.NewWriter(zipped)

	for _, key := range keys {
		name := key
		if base := path.Dir(sources[0]); base != "." {
			name = strings.TrimPrefix(name, base+"/")
		}

		content := mockFileContent(record.Files[key])

		header := &tar.Header{Name: name, Mode: 0600, Size: int64(len(content)), Typeflag: tar.TypeReg}
		if strings.HasSuffix(key, "/") {
			header.Typeflag, header.Mode, header.Size = tar.TypeDir, 0700, 0
		}

		if err := archive.WriteHeader(header); err != nil {
			return err
		}

		if header.Typeflag == tar.TypeReg {
			if _, err := archive.Write([]byte(content)); err != nil {
				return err
			}
		}
	}

	if err := archive.Close(); err != nil {
		return err
	}

	if err := zipped.Close(); err != nil {
		return err
	}

	record.Files[destination] = "\x00archive:" + base64.StdEncoding.EncodeToString(buffer.Bytes())

	return nil
}
