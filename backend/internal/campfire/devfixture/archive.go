//go:build !production

package devfixture

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"maps"
	"math/rand/v2"
	"sort"
	"strings"
	"time"
)

// BackupFiles provides reproducible archive sizes backed by real fixture bytes.
func BackupFiles(files map[string]string, serverID, name string) map[string]string {
	files = maps.Clone(files)
	seed := sha256.Sum256([]byte(serverID + name))

	size := 128*1024 + int(seed[0])*1024
	if name == "mock-before-update.tar.gz" {
		size += 512 * 1024
	}

	data := make([]byte, size)
	_, _ = rand.NewChaCha8(seed).Read(data)
	files["world/"] = ""
	files["world/mock-snapshot-data.txt"] = base64.StdEncoding.EncodeToString(data)

	return files
}

// Archive builds the same downloadable fixture bytes for development and tests.
func Archive(files map[string]string, createdAt time.Time) ([]byte, error) {
	var buffer bytes.Buffer

	zipped := gzip.NewWriter(&buffer)
	archive := tar.NewWriter(zipped)
	names := make([]string, 0, len(files))

	for name := range files {
		names = append(names, name)
	}

	sort.Strings(names)

	for _, name := range names {
		content := files[name]
		header := &tar.Header{Name: name, Mode: 0600, Size: int64(len(content)), ModTime: createdAt}

		if strings.HasSuffix(name, "/") {
			header.Typeflag, header.Mode, header.Size = tar.TypeDir, 0700, 0
		}

		if err := archive.WriteHeader(header); err != nil {
			return nil, err
		}

		if _, err := archive.Write([]byte(content)); err != nil {
			return nil, err
		}
	}

	if err := archive.Close(); err != nil {
		return nil, err
	}

	if err := zipped.Close(); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}
