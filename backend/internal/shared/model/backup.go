package model

import (
	"strings"
	"time"

	"campfire/internal/shared/utils"
)

// BackupPolicy selects storage and opt-in retention for one server.
type BackupPolicy struct {
	Savedata         BackupRetention `json:"savedata"`
	Storage          string          `json:"storage"`
	LocalKeepLast    int             `json:"local_keep_last"`
	LocalMaxAgeDays  int             `json:"local_max_age_days"`
	RemoteKeepLast   int             `json:"remote_keep_last"`
	RemoteMaxAgeDays int             `json:"remote_max_age_days"`
}

// BackupRetention limits local and remote copies of one backup mode.
type BackupRetention struct {
	LocalKeepLast    int `json:"local_keep_last"`
	LocalMaxAgeDays  int `json:"local_max_age_days"`
	RemoteKeepLast   int `json:"remote_keep_last"`
	RemoteMaxAgeDays int `json:"remote_max_age_days"`
}

// Retention preserves the original policy fields as the full-backup limits.
func (p BackupPolicy) Retention(mode BackupMode) BackupRetention {
	if mode.Normalized() == BackupModeSavedata {
		return p.Savedata
	}

	return BackupRetention{LocalKeepLast: p.LocalKeepLast, LocalMaxAgeDays: p.LocalMaxAgeDays, RemoteKeepLast: p.RemoteKeepLast, RemoteMaxAgeDays: p.RemoteMaxAgeDays}
}

// BackupSource grants one bounded download for an off-host restore. It is never
// stored in server configuration, status, audit details or execution history.
type BackupSource struct {
	URL    string `json:"url"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
}

// Backup describes a cataloged archive and its available copies.
type Backup struct {
	Mode       BackupMode `json:"mode"`
	Name       string     `json:"name"`
	Size       int64      `json:"size"`
	ModifiedAt time.Time  `json:"modified_at"`
	Local      bool       `json:"local"`
	Remote     bool       `json:"remote"`
	State      string     `json:"state"`
	Error      string     `json:"error,omitempty"`
}

// CatalogBackup identifies an archive and its owning server in the admin catalog.
type CatalogBackup struct {
	Backup
	ServerID   string `json:"server_id"`
	ServerName string `json:"server_name"`
}

// BackupName restricts archives to immutable operation identifiers.
func BackupName(name string) bool {
	if !strings.HasSuffix(name, ".tar.gz") {
		return false
	}

	id := strings.TrimSuffix(name, ".tar.gz")
	if utils.IsValidResourceID(id) {
		return true
	}

	for _, mode := range []BackupMode{BackupModeFull, BackupModeSavedata} {
		if suffix, ok := strings.CutPrefix(id, string(mode)+"-"); ok && utils.IsValidResourceID(suffix) {
			return true
		}
	}

	return false
}
