package database

// StatusPage is an administrator-managed publication, with no owner or draft state.
type StatusPage struct {
	ID      string            `gorm:"type:text;primaryKey"`
	Title   string            `gorm:"type:text;not null"`
	Slug    string            `gorm:"type:text;not null;uniqueIndex"`
	Entries []StatusPageEntry `gorm:"foreignKey:PageID;constraint:OnDelete:CASCADE"`
}

// StatusPageEntry binds a public name to an explicitly selected server.
type StatusPageEntry struct {
	ID       string `gorm:"type:text;primaryKey"`
	PageID   string `gorm:"type:text;not null;uniqueIndex:status_page_server"`
	ServerID string `gorm:"type:text;not null;index;uniqueIndex:status_page_server"`
	Server   Server `gorm:"foreignKey:ServerID;constraint:OnDelete:CASCADE"`
	Name     string `gorm:"type:text;not null"`
	Position int    `gorm:"not null"`
}
