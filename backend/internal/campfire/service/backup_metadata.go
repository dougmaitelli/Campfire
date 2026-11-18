package service

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/limits"
	"campfire/internal/shared/model"
)

// Update retains the server-scoped endpoint while resolving stable identity.
func (s *BackupService) Update(ctx context.Context, id, name string, input model.BackupChanges) (model.Backup, error) {
	row, err := s.Store.Get(ctx, id, name)
	if err != nil {
		return model.Backup{}, err
	}

	return s.UpdateArchive(ctx, row, input)
}

// UpdateArchive edits metadata without changing archive identity or association.
func (s *BackupService) UpdateArchive(ctx context.Context, expected database.Backup, input model.BackupChanges) (model.Backup, error) {
	if input.DisplayName == nil && input.Pinned == nil {
		return model.Backup{}, &model.FileError{Code: model.FileInvalid, Message: "Select a name or pin change."}
	}

	changes := make(map[string]any)

	if input.DisplayName != nil {
		label := strings.TrimSpace(*input.DisplayName)
		if label == "" || !utf8.ValidString(label) || utf8.RuneCountInString(label) > limits.ResourceNameLength || strings.ContainsFunc(label, unicode.IsControl) {
			return model.Backup{}, &model.FileError{Code: model.FileInvalid, Message: "Backup names must contain 1–100 characters without control characters."}
		}

		changes["display_name"] = label
	}

	if input.Pinned != nil {
		changes["pinned"] = *input.Pinned
	}

	row, err := s.Store.ByID(ctx, expected.ID)
	if err != nil {
		return model.Backup{}, err
	}

	if row.Deleting || row.LocalDeleting || row.RemoteDeleting {
		return model.Backup{}, &model.FileError{Code: model.FileBusy, Message: "Backup deletion is already pending."}
	}

	if !row.Local && !row.Remote {
		return model.Backup{}, &model.FileError{Code: model.FileNotFound, Message: "Backup has no available copies."}
	}

	if err := s.Store.UpdateArchiveMetadata(ctx, expected, changes); err != nil {
		return model.Backup{}, err
	}

	row, err = s.Store.ByID(ctx, expected.ID)
	s.mu.Lock()
	uploading := s.uploading[row.ServerID] == row.Name
	s.mu.Unlock()

	return backupView(row, uploading), err
}
