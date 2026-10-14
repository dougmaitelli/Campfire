package database

// Spark is the persistence model; ORM tags stay outside the domain model.
type Spark struct {
	ID   string `gorm:"type:text;primaryKey"`
	Name string `gorm:"type:text COLLATE NOCASE;not null;uniqueIndex:sparks_name"`
	Kind string `gorm:"type:text;not null;check:sparks_kind,kind IN ('local','remote');uniqueIndex:sparks_one_local,where:kind = 'local'"`
	// Retain RFC3339 text storage for compatibility with existing databases.
	CreatedAt string `gorm:"type:text;not null;autoCreateTime:false"`
}

type schemaMigration struct {
	Version int `gorm:"primaryKey;autoIncrement:false"`
}
