//go:build !production

package devfixture

import (
	"context"
	"time"

	"campfire/internal/campfire/database"
	accessperm "campfire/internal/shared/permissions"

	"gorm.io/gorm"
)

// SeedObserver adds a schedule-viewing account without changing an existing account.
func SeedObserver(ctx context.Context, db *gorm.DB, now time.Time) error {
	tx := db.WithContext(ctx)

	var count int64
	if err := tx.Unscoped().Model(&database.User{}).Where("lower(username) = ?", "observer").Count(&count).Error; err != nil || count > 0 {
		return err
	}

	var admin database.User
	if err := tx.Take(&admin, 1).Error; err != nil {
		return err
	}

	role := database.Role{Name: "Observers", Scope: "server"}
	for _, action := range accessperm.Expand([]string{accessperm.ServerSchedulesRead}) {
		role.Permissions = append(role.Permissions, database.RolePermission{Permission: action})
	}

	if err := tx.Where("lower(name) = ? AND scope = ?", "observers", "server").FirstOrCreate(&role).Error; err != nil {
		return err
	}

	user := database.User{Username: "observer", Name: "Schedule Observer", Email: "observer@example.test", RoleID: database.UserRole, PasswordHash: admin.PasswordHash, CredentialVersion: 1, CreatedAt: now.Unix()}
	if err := tx.Omit("Role").Create(&user).Error; err != nil {
		return err
	}

	if err := tx.Model(&database.Server{}).Where("id = ?", "survival").Count(&count).Error; err != nil || count == 0 {
		return err
	}

	return tx.Omit("Server", "User", "Role", "AccessRole").Create(&database.ServerAccessGrant{ServerID: "survival", UserID: &user.ID, AccessRoleID: role.ID}).Error
}
