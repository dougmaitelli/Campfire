package database

import "campfire/internal/model"

// Spark is the persistence model; ORM tags stay outside the domain model.
type Spark struct {
	ID              string                 `gorm:"type:text;primaryKey"`
	Name            string                 `gorm:"type:text COLLATE NOCASE;not null;uniqueIndex:sparks_name"`
	Kind            string                 `gorm:"type:text;not null;check:sparks_kind,kind IN ('local','remote');uniqueIndex:sparks_one_local,where:kind = 'local'"`
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
