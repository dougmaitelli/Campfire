package service

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	"campfire/internal/shared/limits"
	"campfire/internal/shared/model"
)

// Update edits names and retention protection on the panel, including offline
// archives and ongoing uploads. Conditional writes fence deletion claims.
func (s *BackupService) Update(ctx context.Context, id, name string, input model.BackupChanges) (model.Backup, error) {
	if !model.BackupName(name) || input.DisplayName == nil && input.Pinned == nil {
		return model.Backup{}, &model.FileError{Code: model.FileInvalid, Message: "Select a backup and a name or pin change."}
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

	row, err := s.Store.Get(ctx, id, name)
	if err != nil {
		return model.Backup{}, err
	}

	if row.Deleting || row.LocalDeleting || row.RemoteDeleting {
		return model.Backup{}, &model.FileError{Code: model.FileBusy, Message: "Backup deletion is already pending."}
	}

	if !row.Local && !row.Remote {
		return model.Backup{}, &model.FileError{Code: model.FileNotFound, Message: "Backup has no available copies."}
	}

	if err := s.Store.UpdateMetadata(ctx, id, name, changes); err != nil {
		return model.Backup{}, err
	}

	row, err = s.Store.Get(ctx, id, name)

	s.mu.Lock()
	working := s.active[id]
	s.mu.Unlock()

	return backupView(row, working), err
}
