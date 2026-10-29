package database

import (
	"gorm.io/gorm"

	accessperm "campfire/internal/shared/permissions"
)

func migratePermissions(tx *gorm.DB) error {
	var roles []Role
	if err := tx.Preload("Permissions").Find(&roles).Error; err != nil {
		return err
	}

	for _, role := range roles {
		if role.Scope != "server" {
			continue
		}

		actions := make([]string, 0, len(role.Permissions))
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
