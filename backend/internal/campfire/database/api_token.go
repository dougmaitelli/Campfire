package database

// APIToken stores a personal automation credential. Only its creation response
// contains the secret; persistence and metadata responses never expose it.
type APIToken struct {
	ID                int64    `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID            int64    `gorm:"not null;index" json:"-"`
	User              User     `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	Hash              string   `gorm:"type:text;not null;uniqueIndex" json:"-"`
	CredentialVersion int64    `gorm:"not null" json:"-"`
	Name              string   `gorm:"type:text;not null" json:"name"`
	Prefix            string   `gorm:"type:text;not null" json:"prefix"`
	Scopes            []string `gorm:"serializer:json;type:text;not null" json:"scopes"`
	CreatedAt         int64    `gorm:"not null;autoCreateTime:false" json:"created_at"`
	ExpiresAt         int64    `gorm:"not null;index" json:"expires_at"`
	LastUsedAt        int64    `gorm:"not null;default:0" json:"last_used_at"`
}
