package database

// UserMFA stores only encrypted-secret references and hashed recovery codes.
// It is never serialized in account or authentication responses.
type UserMFA struct {
	UserID         int64       `gorm:"primaryKey;autoIncrement:false"`
	User           User        `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`
	SecretRef      string      `gorm:"type:text;not null;default:''"`
	RecoveryHashes []string    `gorm:"serializer:json;type:text;not null;default:'[]'"`
	LastCounter    int64       `gorm:"not null;default:0"`
	PendingRef     string      `gorm:"type:text;not null;default:''"`
	PendingSession string      `gorm:"type:text;not null;default:''"`
	PendingExpires int64       `gorm:"not null;default:0"`
	PendingName    string      `gorm:"type:text;not null;default:''"`
	Devices        []MFADevice `gorm:"serializer:json;type:text;not null;default:'[]'"`
}

// MFADevice stores a private encrypted-secret reference and independent replay state.
type MFADevice struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	SecretRef   string `json:"secret_ref"`
	LastCounter int64  `json:"last_counter"`
	CreatedAt   int64  `json:"created_at"`
}
