//go:build e2e && !production

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"strings"

	"campfire/internal/campfire/devfixture"
	"campfire/internal/shared/backuparchive"
	"campfire/internal/shared/model"
)

func (r *runtime) createBackup(input model.RuntimeRequest) ([]byte, error) {
	files := maps.Clone(r.files[input.ServerID])
	options := input.BackupOptions
	if options.Mode == model.BackupModeSavedata {
		maps.DeleteFunc(files, func(name, _ string) bool { return !options.Includes(strings.TrimSuffix(name, "/")) })
	}
	manifest := backuparchive.Manifest{Format: 1, BackupOptions: options}
	if options.Definition != nil {
		manifest.Format = backuparchive.Format
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}
	files[model.BackupMetadataName] = string(data)
	return devfixture.Archive(files, fixed)
}

func (r *runtime) importBackup(ctx context.Context, input model.FileRequest) (model.FileResult, error) {
	result := model.FileResult{ServerID: input.ServerID, Path: input.Path, UploadID: input.UploadID}
	if r.uploads == nil {
		r.uploads = make(map[string]*uploadFixture)
	}
	if input.Operation == model.FileOperationBackupUploadBegin {
		result.UploadID = rand.Text()
		r.uploads[result.UploadID] = &uploadFixture{serverID: input.ServerID, path: input.Path, size: input.Size}
		return result, nil
	}
	upload := r.uploads[input.UploadID]
	if upload == nil || upload.serverID != input.ServerID || upload.path != input.Path {
		return result, &model.FileError{Code: model.FileNotFound, Message: "Upload not found."}
	}
	switch input.Operation {
	case model.FileOperationBackupUploadChunk:
		if input.Offset != int64(len(upload.data)) || int64(len(input.Data)) > upload.size-input.Offset {
			return result, &model.FileError{Code: model.FileInvalid, Message: "Invalid chunk."}
		}
		upload.data = append(upload.data, input.Data...)
		result.Size = int64(len(upload.data))
	case model.FileOperationBackupUploadCommit:
		manifest, err := backuparchive.Inspect(ctx, bytes.NewReader(upload.data))
		if err != nil {
			return result, err
		}
		r.archives[input.ServerID+"/"+input.Path] = upload.data
		r.backups[input.ServerID] = append(r.backups[input.ServerID], model.FileEntry{Portable: true, BackupMode: manifest.Mode, Name: input.Path, Kind: model.FileKindFile, Size: int64(len(upload.data)), ModifiedAt: fixed})
		delete(r.uploads, input.UploadID)
	case model.FileOperationBackupUploadCancel:
		delete(r.uploads, input.UploadID)
	}
	return result, nil
}

func (r *runtime) restoreBackup(input model.RuntimeRequest) error {
	data := r.archives[input.ServerID+"/"+input.Content]
	manifest, err := backuparchive.Read(bytes.NewReader(data))
	if err != nil {
		return err
	}
	compressed, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer func() { _ = compressed.Close() }()
	files := make(map[string]string)
	if manifest.Mode == model.BackupModeSavedata {
		for name, content := range r.files[input.ServerID] {
			if !manifest.Includes(strings.TrimSuffix(name, "/")) {
				files[name] = content
			}
		}
	}
	archive := tar.NewReader(compressed)
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if header.Name == model.BackupMetadataName {
			continue
		}
		data, err := io.ReadAll(archive)
		if err != nil {
			return err
		}
		files[header.Name] = string(data)
	}
	r.files[input.ServerID] = files
	return nil
}
