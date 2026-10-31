package model

import (
	"errors"
	"path"
	"strings"
	"unicode"
)

// BackupMode selects the contents of one backup, independently of server settings.
type BackupMode string

// Backup modes distinguish complete installations from declared save data.
const (
	BackupModeFull     BackupMode = "full"
	BackupModeSavedata BackupMode = "savedata"
)

// Normalized treats backups created before modes existed as full snapshots.
func (m BackupMode) Normalized() BackupMode {
	if m == "" {
		return BackupModeFull
	}

	return m
}

// ArchiveName makes new snapshots recognizable without opening their metadata.
func (m BackupMode) ArchiveName(id string) string {
	return string(m.Normalized()) + "-" + id + ".tar.gz"
}

// SavedataPath selects a literal file or recursive directory below the game-data root.
type SavedataPath struct {
	Path     string `json:"path"`
	Required bool   `json:"required"`
}

// BackupOptions carries resolved execution paths, rather than the Ember definition.
type BackupOptions struct {
	Mode          BackupMode     `json:"mode"`
	Paths         []SavedataPath `json:"paths,omitempty"`
	EmberRevision string         `json:"ember_revision,omitempty"`
}

// BackupMetadataName is reserved inside backup archives for their restore contract.
const BackupMetadataName = ".campfire-backup.json"

// ValidateSavedataPaths rejects ambiguous roots and paths that could escape game data.
func ValidateSavedataPaths(paths []SavedataPath) error {
	if len(paths) > 64 {
		return errors.New("savedata_paths supports at most 64 paths")
	}

	for i, item := range paths {
		p := item.Path
		if p == "" || p == "." || len(p) > 1024 || path.Clean(p) != p || !SafePath(p) || strings.ContainsAny(p, "\\*?[]{}$") || strings.ContainsFunc(p, unicode.IsControl) || p == BackupMetadataName || strings.HasPrefix(p, BackupMetadataName+"/") {
			return errors.New("savedata_paths requires literal relative paths below the game-data root")
		}

		for _, other := range paths[:i] {
			if p == other.Path || strings.HasPrefix(p, other.Path+"/") || strings.HasPrefix(other.Path, p+"/") {
				return errors.New("savedata_paths cannot contain duplicate or overlapping paths")
			}
		}
	}

	return nil
}

// Includes reports whether a name belongs to a selected root.
func (o BackupOptions) Includes(name string) bool {
	if o.Mode.Normalized() == BackupModeFull {
		return true
	}

	for _, item := range o.Paths {
		if name == item.Path || strings.HasPrefix(name, item.Path+"/") {
			return true
		}
	}

	return false
}

// Validate checks a backup request independently at the panel and Spark boundaries.
func (o BackupOptions) Validate() error {
	switch o.Mode.Normalized() {
	case BackupModeFull:
		if len(o.Paths) != 0 {
			return errors.New("full backups cannot specify selected paths")
		}
	case BackupModeSavedata:
		if len(o.Paths) == 0 {
			return errors.New("save-data backups require Ember savedata_paths")
		}
	default:
		return errors.New("invalid backup mode")
	}

	return ValidateSavedataPaths(o.Paths)
}
