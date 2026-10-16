package model

import (
	"strings"
	"time"
)

// BackupPolicy selects storage and opt-in retention for one server.
type BackupPolicy struct {
	Storage          string `json:"storage"`
	LocalKeepLast    int    `json:"local_keep_last"`
	LocalMaxAgeDays  int    `json:"local_max_age_days"`
	RemoteKeepLast   int    `json:"remote_keep_last"`
	RemoteMaxAgeDays int    `json:"remote_max_age_days"`
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
	Name       string    `json:"name"`
	Size       int64     `json:"size"`
	ModifiedAt time.Time `json:"modified_at"`
	Local      bool      `json:"local"`
	Remote     bool      `json:"remote"`
	State      string    `json:"state"`
	Error      string    `json:"error,omitempty"`
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
	if len(id) < 1 || len(id) > 128 {
		return false
	}

	for _, c := range id {
		valid := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-'
		if !valid {
			return false
		}
	}

	return true
}
