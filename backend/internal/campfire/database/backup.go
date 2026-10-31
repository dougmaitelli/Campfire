package database

import "campfire/internal/shared/model"

// BackupPolicy holds opt-in retention and the chosen storage backend.
type BackupPolicy struct {
	SavedataLocalKeepLast    int    `gorm:"not null;default:0"`
	SavedataLocalMaxAgeDays  int    `gorm:"not null;default:0"`
	SavedataRemoteKeepLast   int    `gorm:"not null;default:0"`
	SavedataRemoteMaxAgeDays int    `gorm:"not null;default:0"`
	ServerID                 string `gorm:"type:text;primaryKey"`
	Storage                  string `gorm:"type:text;not null"`
	LocalKeepLast            int    `gorm:"not null"`
	LocalMaxAgeDays          int    `gorm:"not null"`
	RemoteKeepLast           int    `gorm:"not null"`
	RemoteMaxAgeDays         int    `gorm:"not null"`
}

// Backup retains archive identity across Spark moves and panel restarts.
// Deleting is a durable tombstone until all copies have been removed.
type Backup struct {
	Mode           model.BackupMode `gorm:"type:text;not null;default:'full'"`
	ServerID       string           `gorm:"type:text;primaryKey"`
	Name           string           `gorm:"type:text;primaryKey"`
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
