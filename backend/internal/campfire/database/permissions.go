package database

import (
	"campfire/internal/shared/model"
	accessperm "campfire/internal/shared/permissions"

	"gorm.io/gorm"
)

func migratePermissions(tx *gorm.DB) error {
	var roles []Role
	if err := tx.Preload("Permissions").Find(&roles).Error; err != nil {
		return err
	}

	for _, role := range roles {
		if role.Scope != model.RoleScopeServer {
			continue
		}

		actions := make([]model.PermissionID, 0, len(role.Permissions))
		for _, item := range role.Permissions {
			actions = append(actions, item.Permission)
		}

		actions = accessperm.Legacy(actions)
		if role.ID == OwnerRole {
			actions = accessperm.All()
		}

		if err := tx.Where("role_id = ?", role.ID).Delete(&RolePermission{}).Error; err != nil {
			return err
		}

		for _, action := range actions {
			if err := tx.Create(&RolePermission{RoleID: role.ID, Permission: action}).Error; err != nil {
				return err
			}
		}
	}

	var tokens []APIToken
	if err := tx.Find(&tokens).Error; err != nil {
		return err
	}

	for _, token := range tokens {
		token.Scopes = accessperm.Legacy(token.Scopes)
		if err := tx.Model(&token).Select("Scopes").Updates(&token).Error; err != nil {
			return err
		}
	}

	return nil
}
