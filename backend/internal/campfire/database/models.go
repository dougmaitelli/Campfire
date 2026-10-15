package database

import "campfire/internal/shared/model"

// Spark is the persistence model; ORM tags stay outside the domain model.
type Spark struct {
	ID              string                 `gorm:"type:text;primaryKey"`
	Name            string                 `gorm:"type:text COLLATE NOCASE;not null;uniqueIndex:sparks_name"`
	Kind            string                 `gorm:"type:text;not null;check:sparks_kind,kind = 'remote'"`
	Description     string                 `gorm:"type:text;not null;default:''"`
	Disabled        bool                   `gorm:"not null;default:false"`
	GameAddress     string                 `gorm:"type:text;not null;default:''"`
	CPULimit        float64                `gorm:"not null;default:0"`
	MemoryMB        int64                  `gorm:"not null;default:0"`
	DiskMB          int64                  `gorm:"not null;default:0"`
	PortAllocations []model.PortAllocation `gorm:"serializer:json;type:text;not null;default:'[]'"`
	// Retain RFC3339 text storage for compatibility with existing databases.
	CreatedAt string `gorm:"type:text;not null;autoCreateTime:false"`
}

type schemaMigration struct {
	Version int `gorm:"primaryKey;autoIncrement:false"`
}

// AdminCredential is the single administrator; the database enforces its singleton ID.
type AdminCredential struct {
	ID           int    `gorm:"primaryKey;autoIncrement:false;check:admin_singleton,id = 1"`
	Username     string `gorm:"type:text;not null"`
	PasswordHash string `gorm:"type:text;not null"`
	Version      int64  `gorm:"not null"`
}

// AdminSession persists hashed session identities and bounded lifetimes.
type AdminSession struct {
	Hash              string `gorm:"type:text;primaryKey"`
	CSRF              string `gorm:"type:text;not null"`
	CredentialVersion int64  `gorm:"not null"`
	ExpiresAt         int64  `gorm:"not null;index"`
	LastSeenAt        int64  `gorm:"not null"`
}

// Server persists server configuration with pinned Spark and Ember identities.
type Server struct {
	ID          string             `gorm:"type:text;primaryKey"`
	Name        string             `gorm:"type:text COLLATE NOCASE;not null;uniqueIndex:servers_name"`
	Description string             `gorm:"type:text;not null;default:''"`
	SparkID     string             `gorm:"type:text;not null;index"`
	Spark       Spark              `gorm:"foreignKey:SparkID;constraint:OnDelete:RESTRICT"`
	EmberID     string             `gorm:"type:text;not null;index"`
	Ember       Ember              `gorm:"foreignKey:EmberID;constraint:OnDelete:RESTRICT"`
	Image       string             `gorm:"type:text;not null"`
	Variables   map[string]string  `gorm:"serializer:json;type:text;not null"`
	CPULimit    float64            `gorm:"not null"`
	MemoryMB    int64              `gorm:"not null"`
	DiskMB      int64              `gorm:"not null"`
	Ports       []model.ServerPort `gorm:"serializer:json;type:text;not null"`
	CreatedAt   string             `gorm:"type:text;not null;autoCreateTime:false"`
}

// Ember stores an immutable imported game definition.
type Ember struct {
	ID         string      `gorm:"type:text;primaryKey"`
	Name       string      `gorm:"type:text;not null;index"`
	Definition model.Ember `gorm:"serializer:json;type:text;not null"`
	CreatedAt  string      `gorm:"type:text;not null;autoCreateTime:false"`
}

// SparkEnrollment stores only a short-lived enrollment token hash.
type SparkEnrollment struct {
	Hash      string `gorm:"type:text;primaryKey"`
	SparkID   string `gorm:"type:text;not null;uniqueIndex"`
	Spark     Spark  `gorm:"foreignKey:SparkID;constraint:OnDelete:CASCADE"`
	ExpiresAt int64  `gorm:"not null"`
}

// SparkIdentity records the currently authorized client certificate.
type SparkIdentity struct {
	SparkID         string `gorm:"type:text;primaryKey"`
	Spark           Spark  `gorm:"foreignKey:SparkID;constraint:OnDelete:CASCADE"`
	CertificateHash string `gorm:"type:text;not null"`
}
