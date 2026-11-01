// Package backuparchive defines the portable backup archive contract.
package backuparchive

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path"
	"strings"

	"campfire/internal/shared/model"
)

// Archive limits bound metadata allocation and expanded upload validation.
const (
	Format              = 2
	MetadataLimit       = 2 << 20
	EntryLimit          = 100000
	ExpandedLimit int64 = 100 << 30
)

// Manifest records both restore selection and the inputs needed for recreation.
type Manifest struct {
	Format int `json:"format"`
	model.BackupOptions
	Missing []string `json:"missing,omitempty"`
}

type contextReader struct {
	ctx context.Context
	io.Reader
}

func (r contextReader) Read(data []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}

	return r.Reader.Read(data)
}

// Validate checks the captured selection and portable authoring inputs.
func (m Manifest) Validate() error {
	if (m.Format != 1 && m.Format != Format) || m.Mode == "" {
		return errors.New("unsupported backup metadata format")
	}

	if err := m.BackupOptions.Validate(); err != nil {
		return err
	}

	if m.Format == Format {
		if m.Definition == nil {
			return errors.New("portable backup definition is missing")
		}

		definition := m.Definition
		if err := definition.Ember.Validate(); err != nil {
			return errors.New("invalid backup Ember definition")
		}

		if definition.Configuration.SparkID != "" || len(definition.Configuration.Ports) != 0 {
			return errors.New("portable backups cannot specify destination placement")
		}

		if _, err := model.NewDeployment("backup", definition.Configuration, definition.Ember); err != nil {
			return errors.New("invalid backup server configuration")
		}
	}

	seen := make(map[string]bool)

	for _, name := range m.Missing {
		optional := false
		for _, item := range m.Paths {
			optional = optional || item.Path == name && !item.Required
		}

		if !optional || seen[name] {
			return errors.New("invalid missing save-data path")
		}

		seen[name] = true
	}

	return nil
}

// Read reads only the first entry. Legacy archives remain restorable, but are
// not accepted by Inspect for the portable import workflow.
func Read(input io.Reader) (Manifest, error) {
	manifest := Manifest{Format: 1, BackupOptions: model.BackupOptions{Mode: model.BackupModeFull}}

	compressed, err := gzip.NewReader(input)
	if err != nil {
		return manifest, err
	}

	defer func() { _ = compressed.Close() }()

	archive := tar.NewReader(compressed)

	header, err := archive.Next()
	if errors.Is(err, io.EOF) {
		return manifest, nil
	}

	if err != nil {
		return manifest, err
	}

	if header.Name != model.BackupMetadataName {
		return manifest, nil
	}

	return Decode(archive, header)
}

// Decode bounds metadata before allocating or decoding authoring inputs.
func Decode(input io.Reader, header *tar.Header) (Manifest, error) {
	var manifest Manifest
	if header.Typeflag != tar.TypeReg || header.Size < 1 || header.Size > MetadataLimit {
		return manifest, errors.New("invalid backup metadata")
	}

	data, err := io.ReadAll(io.LimitReader(input, header.Size))
	if err != nil {
		return manifest, err
	}

	if err := json.Unmarshal(data, &manifest); err != nil {
		return manifest, errors.New("invalid backup metadata")
	}

	return manifest, manifest.Validate()
}

// Inspect validates the entire upload without extracting files. Errors never
// echo metadata, since authoring inputs can contain credentials.
func Inspect(ctx context.Context, input io.Reader) (Manifest, error) {
	var manifest Manifest

	compressed, err := gzip.NewReader(contextReader{ctx: ctx, Reader: input})
	if err != nil {
		return manifest, errors.New("select a Campfire portable .tar.gz backup")
	}

	defer func() { _ = compressed.Close() }()

	archive := tar.NewReader(compressed)

	header, err := archive.Next()
	if err != nil || header.Name != model.BackupMetadataName {
		return manifest, errors.New("portable backup metadata is missing")
	}

	manifest, err = Decode(archive, header)
	if err != nil {
		return manifest, err
	}

	if manifest.Format != Format {
		return manifest, errors.New("this backup predates portable backup support")
	}

	seen := make(map[string]bool)

	var total int64

	for count := 0; ; count++ {
		if err := ctx.Err(); err != nil {
			return manifest, err
		}

		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return manifest, errors.New("invalid backup archive")
		}

		name := strings.TrimSuffix(header.Name, "/")
		if count >= EntryLimit || !model.SafePath(name) || path.Clean(name) != name || name == "." || name == model.BackupMetadataName || strings.Contains(name, "\\") || seen[name] || !manifest.Includes(name) || (header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeDir) || header.Size < 0 || header.Size > ExpandedLimit-total {
			return manifest, errors.New("invalid backup entry or archive limit exceeded")
		}

		for _, missing := range manifest.Missing {
			if name == missing || strings.HasPrefix(name, missing+"/") {
				return manifest, errors.New("backup contains a path recorded as missing")
			}
		}

		seen[name] = true

		total += header.Size
		if _, err := io.CopyN(io.Discard, archive, header.Size); err != nil {
			return manifest, errors.New("incomplete backup archive")
		}
	}

	for _, item := range manifest.Paths {
		found := seen[item.Path]
		for name := range seen {
			found = found || strings.HasPrefix(name, item.Path+"/")
		}

		missing := false
		for _, name := range manifest.Missing {
			missing = missing || name == item.Path
		}

		if found == missing {
			return manifest, errors.New("backup is missing a captured save-data root")
		}
	}

	if n, err := io.Copy(io.Discard, io.LimitReader(compressed, model.FileLimit)); err != nil || n == model.FileLimit {
		return manifest, errors.New("backup checksum or trailing data is invalid")
	}

	return manifest, nil
}
