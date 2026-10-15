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

// AdminCredential describes the retired schema solely for version-six migrations.
type AdminCredential struct {
	ID           int    `gorm:"primaryKey;autoIncrement:false;check:admin_singleton,id = 1"`
	Username     string `gorm:"type:text;not null"`
	PasswordHash string `gorm:"type:text;not null"`
	Version      int64  `gorm:"not null"`
}

// AdminSession describes retired sessions that are revoked by the multi-user migration.
type AdminSession struct {
	Hash              string `gorm:"type:text;primaryKey"`
	CSRF              string `gorm:"type:text;not null"`
	CredentialVersion int64  `gorm:"not null"`
	ExpiresAt         int64  `gorm:"not null;index"`
	LastSeenAt        int64  `gorm:"not null"`
}

// Role defines platform or server permissions. Built-in roles cannot be changed.
type Role struct {
	ID          int64            `gorm:"primaryKey;autoIncrement" json:"id"`
	Name        string           `gorm:"type:text COLLATE NOCASE;not null;uniqueIndex:roles_name_scope" json:"name"`
	Scope       string           `gorm:"type:text;not null;check:roles_scope,scope IN ('platform','server');uniqueIndex:roles_name_scope" json:"scope"`
	Builtin     bool             `gorm:"not null" json:"builtin"`
	Permissions []RolePermission `gorm:"foreignKey:RoleID;constraint:OnDelete:CASCADE" json:"-"`
}

// RolePermission grants one action to a role.
type RolePermission struct {
	RoleID     int64  `gorm:"primaryKey;autoIncrement:false"`
	Permission string `gorm:"type:text;primaryKey"`
}

// User stores a panel account with one platform role.
type User struct {
	ID           int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	Username     string `gorm:"type:text COLLATE NOCASE;not null;uniqueIndex:users_username" json:"username"`
	PasswordHash string `gorm:"type:text;not null" json:"-"`
	Version      int64  `gorm:"not null" json:"-"`
	RoleID       int64  `gorm:"not null;index" json:"role_id"`
	Role         Role   `gorm:"foreignKey:RoleID;constraint:OnDelete:RESTRICT" json:"role"`
	Disabled     bool   `gorm:"not null;default:false" json:"disabled"`
	CreatedAt    int64  `gorm:"not null;autoCreateTime" json:"created_at"`
}

// UserSession binds a hashed session to an account and credential generation.
type UserSession struct {
	Hash              string `gorm:"type:text;primaryKey"`
	UserID            int64  `gorm:"not null;index"`
	User              User   `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`
	CSRF              string `gorm:"type:text;not null"`
	CredentialVersion int64  `gorm:"not null"`
	ExpiresAt         int64  `gorm:"not null;index"`
	LastSeenAt        int64  `gorm:"not null"`
}

// ServerOwnership identifies the single owner; account removal requires transfer first.
type ServerOwnership struct {
	ServerID string `gorm:"type:text;primaryKey"`
	UserID   int64  `gorm:"not null;index"`
	Server   Server `gorm:"foreignKey:ServerID;constraint:OnDelete:CASCADE"`
	User     User   `gorm:"foreignKey:UserID;constraint:OnDelete:RESTRICT"`
}

// ServerMember grants scoped access to an additional user.
type ServerMember struct {
	ServerID string `gorm:"type:text;primaryKey"`
	UserID   int64  `gorm:"primaryKey;autoIncrement:false"`
	RoleID   int64  `gorm:"not null"`
	Server   Server `gorm:"foreignKey:ServerID;constraint:OnDelete:CASCADE"`
	User     User   `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`
	Role     Role   `gorm:"foreignKey:RoleID;constraint:OnDelete:RESTRICT"`
}

// SparkRole permits a platform role to provision servers on a Spark.
type SparkRole struct {
	SparkID string `gorm:"type:text;primaryKey"`
	RoleID  int64  `gorm:"primaryKey;autoIncrement:false"`
	Spark   Spark  `gorm:"foreignKey:SparkID;constraint:OnDelete:CASCADE"`
	Role    Role   `gorm:"foreignKey:RoleID;constraint:OnDelete:CASCADE"`
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
