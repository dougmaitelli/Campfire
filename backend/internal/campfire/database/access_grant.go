package database

// ServerAccessGrant assigns an access role to either a user or an account role.
type ServerAccessGrant struct {
	ID           int64  `gorm:"primaryKey;autoIncrement"`
	ServerID     string `gorm:"type:text;not null;uniqueIndex:server_access_user,priority:1;uniqueIndex:server_access_role,priority:1"`
	Server       Server `gorm:"foreignKey:ServerID;constraint:OnDelete:CASCADE"`
	UserID       *int64 `gorm:"uniqueIndex:server_access_user,priority:2;check:server_access_subject,(user_id IS NOT NULL AND role_id IS NULL) OR (user_id IS NULL AND role_id IS NOT NULL)"`
	User         *User  `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`
	RoleID       *int64 `gorm:"uniqueIndex:server_access_role,priority:2"`
	Role         *Role  `gorm:"foreignKey:RoleID;constraint:OnDelete:CASCADE"`
	AccessRoleID int64  `gorm:"not null;index"`
	AccessRole   Role   `gorm:"foreignKey:AccessRoleID;constraint:OnDelete:RESTRICT"`
}

// SparkAccessGrant uses the same subjects and access-role structure for provisioning.
type SparkAccessGrant struct {
	ID           int64  `gorm:"primaryKey;autoIncrement"`
	SparkID      string `gorm:"type:text;not null;uniqueIndex:spark_access_user,priority:1;uniqueIndex:spark_access_role,priority:1"`
	Spark        Spark  `gorm:"foreignKey:SparkID;constraint:OnDelete:CASCADE"`
	UserID       *int64 `gorm:"uniqueIndex:spark_access_user,priority:2;check:spark_access_subject,(user_id IS NOT NULL AND role_id IS NULL) OR (user_id IS NULL AND role_id IS NOT NULL)"`
	User         *User  `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`
	RoleID       *int64 `gorm:"uniqueIndex:spark_access_role,priority:2"`
	Role         *Role  `gorm:"foreignKey:RoleID;constraint:OnDelete:CASCADE"`
	AccessRoleID int64  `gorm:"not null;index"`
	AccessRole   Role   `gorm:"foreignKey:AccessRoleID;constraint:OnDelete:RESTRICT"`
}
