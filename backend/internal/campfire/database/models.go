package database

import (
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

// Spark is the persistence model; ORM tags stay outside the domain model.
type Spark struct {
	ID              string                 `gorm:"type:text;primaryKey"`
	Name            string                 `gorm:"type:text;not null;uniqueIndex:sparks_name,expression:lower(name)"`
	Description     string                 `gorm:"type:text;not null;default:''"`
	Disabled        bool                   `gorm:"not null;default:false"`
	GameAddress     string                 `gorm:"type:text;not null;default:''"`
	CPULimit        float64                `gorm:"not null;default:0"`
	MemoryMB        int64                  `gorm:"not null;default:0"`
	DiskMB          int64                  `gorm:"not null;default:0"`
	PortAllocations []model.PortAllocation `gorm:"serializer:json;type:text;not null;default:'[]'"`
	// Retain RFC3339 text storage for compatibility with existing databases.
	CreatedAt           string  `gorm:"type:text;not null;autoCreateTime:false"`
	EnrollmentTokenHash *string `gorm:"type:text;uniqueIndex"`
	EnrollmentExpiresAt int64   `gorm:"not null;default:0"`
}

type schemaMigration struct {
	Version int `gorm:"primaryKey;autoIncrement:false"`
}

// Role defines platform or server permissions. Built-in roles cannot be changed.
type Role struct {
	ID          int64            `gorm:"primaryKey;autoIncrement" json:"id"`
	Name        string           `gorm:"type:text;not null;uniqueIndex:roles_name_scope,expression:lower(name)" json:"name"`
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
	ID                int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	Username          string         `gorm:"type:text;not null;uniqueIndex:users_username,expression:lower(username)" json:"username"`
	Name              string         `gorm:"type:text;not null;default:''" json:"name"`
	Email             string         `gorm:"type:text;not null;default:''" json:"email"`
	Picture           string         `gorm:"type:text;not null;default:''" json:"picture"`
	OIDCNameManaged   bool           `gorm:"column:oidc_name_managed;not null;default:false" json:"oidc_name_managed"`
	OIDCEmailManaged  bool           `gorm:"column:oidc_email_managed;not null;default:false" json:"oidc_email_managed"`
	PasswordHash      string         `gorm:"type:text;not null" json:"-"`
	CredentialVersion int64          `gorm:"not null" json:"-"`
	RoleID            int64          `gorm:"not null;index" json:"role_id"`
	Role              Role           `gorm:"foreignKey:RoleID;constraint:OnDelete:RESTRICT" json:"role"`
	Disabled          bool           `gorm:"not null;default:false" json:"disabled"`
	CreatedAt         int64          `gorm:"not null;autoCreateTime" json:"created_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

// UserSession binds a hashed session to an account and credential generation.
type UserSession struct {
	Hash              string `gorm:"type:text;primaryKey"`
	Provider          string `gorm:"type:text;not null;default:''"`
	UserID            int64  `gorm:"not null;index"`
	User              User   `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`
	CSRF              string `gorm:"type:text;not null"`
	CredentialVersion int64  `gorm:"not null"`
	ExpiresAt         int64  `gorm:"not null;index"`
	LastSeenAt        int64  `gorm:"not null"`
}

// OIDCIdentity binds a provider subject to an account without matching mutable claims.
type OIDCIdentity struct {
	Issuer  string `gorm:"type:text;primaryKey"`
	Subject string `gorm:"type:text;primaryKey"`
	UserID  int64  `gorm:"not null;index"`
	User    User   `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`
}

// TableName keeps the protocol acronym intact in SQL identifiers.
func (OIDCIdentity) TableName() string { return "oidc_identities" }

// Server persists server configuration with pinned Spark and Ember identities.
type Server struct {
	ID              string             `gorm:"type:text;primaryKey"`
	OwnerUserID     *int64             `gorm:"index"`
	Owner           *User              `gorm:"foreignKey:OwnerUserID;constraint:OnDelete:RESTRICT"`
	Name            string             `gorm:"type:text;not null;uniqueIndex:servers_name,expression:lower(name)"`
	Description     string             `gorm:"type:text;not null;default:''"`
	SparkID         string             `gorm:"type:text;not null;index"`
	Spark           Spark              `gorm:"foreignKey:SparkID;constraint:OnDelete:RESTRICT"`
	EmberID         string             `gorm:"type:text;not null;index"`
	Ember           Ember              `gorm:"foreignKey:EmberID;constraint:OnDelete:RESTRICT"`
	EmberHash       string             `gorm:"type:text;not null;default:''"`
	Image           string             `gorm:"type:text;not null"`
	StartupCommand  string             `gorm:"type:text;not null;default:''"`
	StartupOverride string             `gorm:"type:text;not null;default:''"`
	GameIntegration bool               `gorm:"not null;default:false"`
	Variables       map[string]string  `gorm:"serializer:json;type:text;not null"`
	CPULimit        float64            `gorm:"not null"`
	MemoryMB        int64              `gorm:"not null"`
	DiskMB          int64              `gorm:"not null"`
	Ports           []model.ServerPort `gorm:"serializer:json;type:text;not null"`
	CreatedAt       string             `gorm:"type:text;not null;autoCreateTime:false"`
}

// Ember stores the current imported game definition.
type Ember struct {
	ID              string      `gorm:"type:text;primaryKey"`
	Name            string      `gorm:"type:text;not null;index"`
	Definition      model.Ember `gorm:"serializer:json;type:text;not null"`
	CreatedAt       string      `gorm:"type:text;not null;autoCreateTime:false"`
	UpdateCheckedAt int64       `gorm:"not null;default:0"`
	UpdateAvailable bool        `gorm:"not null;default:false"`
	UpdateError     string      `gorm:"type:text;not null;default:''"`
}

// SparkIdentity records the currently authorized client certificate.
type SparkIdentity struct {
	SparkID                 string `gorm:"type:text;primaryKey"`
	Spark                   Spark  `gorm:"foreignKey:SparkID;constraint:OnDelete:CASCADE"`
	CertificateHash         string `gorm:"type:text;not null"`
	PreviousCertificateHash string `gorm:"type:text;not null;default:''"`
	PreviousExpiresAt       int64  `gorm:"not null;default:0"`
	Certificate             []byte
}
