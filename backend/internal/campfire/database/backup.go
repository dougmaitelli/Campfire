package database

import (
	"crypto/rand"

	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

// BackupPolicy holds opt-in retention and the chosen storage backend.
type BackupPolicy struct {
	SavedataLocalKeepLast    int                 `gorm:"not null;default:0"`
	SavedataLocalMaxAgeDays  int                 `gorm:"not null;default:0"`
	SavedataRemoteKeepLast   int                 `gorm:"not null;default:0"`
	SavedataRemoteMaxAgeDays int                 `gorm:"not null;default:0"`
	ServerID                 string              `gorm:"type:text;primaryKey"`
	Storage                  model.BackupStorage `gorm:"type:text;not null"`
	LocalKeepLast            int                 `gorm:"not null"`
	LocalMaxAgeDays          int                 `gorm:"not null"`
	RemoteKeepLast           int                 `gorm:"not null"`
	RemoteMaxAgeDays         int                 `gorm:"not null"`
}

// Backup retains archive identity across Spark moves and panel restarts.
// Deleting is a durable tombstone until all copies have been removed.
type Backup struct {
	ID string `gorm:"type:text;primaryKey;not null"`
	// Empty ServerID is represented by SQL NULL, not an archive's identity.
	OwnerUserID    *int64
	LocalStore     string           `gorm:"type:text;not null;default:''"`
	LocalKey       string           `gorm:"type:text;not null;default:''"`
	DisplayName    string           `gorm:"type:text;not null;default:''"`
	Pinned         bool             `gorm:"not null;default:false"`
	Portable       bool             `gorm:"not null;default:false"`
	Mode           model.BackupMode `gorm:"type:text;not null;default:'full'"`
	ServerID       string           `gorm:"type:text;default:null;uniqueIndex:idx_backups_server_name"`
	Name           string           `gorm:"type:text;not null;uniqueIndex:idx_backups_server_name"`
	Size           int64            `gorm:"not null"`
	CreatedAt      int64            `gorm:"not null;autoCreateTime:false"`
	Remote         bool             `gorm:"not null"`
	Local          bool             `gorm:"not null"`
	StoreID        string           `gorm:"type:text;not null;default:''"`
	Key            string           `gorm:"type:text;not null"`
	Version        string           `gorm:"type:text;not null;default:''"`
	Digest         string           `gorm:"type:text;not null;default:''"`
	Deleting       bool             `gorm:"not null;default:false"`
	LocalDeleting  bool             `gorm:"not null;default:false"`
	RemoteDeleting bool             `gorm:"not null;default:false"`
	NoRemote       bool             `gorm:"not null;default:false"`
	Error          string           `gorm:"type:text;not null;default:''"`
}

// BeforeCreate assigns an archive identity independently of its optional server.
func (b *Backup) BeforeCreate(_ *gorm.DB) error {
	if b.ID == "" {
		b.ID = rand.Text()
	}

	return nil
}
