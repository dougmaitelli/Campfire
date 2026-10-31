package database

import (
	"campfire/internal/shared/model"
	accessperm "campfire/internal/shared/permissions"

	"gorm.io/gorm"
)

// Built-in role identifiers are stable so authorization does not depend on display names.
const (
	AdminRole    int64 = 1
	CreatorRole  int64 = 2
	UserRole     int64 = 3
	OwnerRole    int64 = 4
	OperatorRole int64 = 5
)

func seedRoles(tx *gorm.DB) error {
	roles := []Role{
		{ID: AdminRole, Name: "Admin", Scope: model.RoleScopePlatform, Builtin: true},
		{ID: CreatorRole, Name: "Server creator", Scope: model.RoleScopePlatform, Builtin: true},
		{ID: UserRole, Name: "User", Scope: model.RoleScopePlatform, Builtin: true},
		{ID: OwnerRole, Name: "Owner", Scope: model.RoleScopeServer, Builtin: true},
		{ID: OperatorRole, Name: "Operator", Scope: model.RoleScopeServer, Builtin: true},
	}
	for _, role := range roles {
		if err := tx.Create(&role).Error; err != nil {
			return err
		}
	}

	permissions := map[int64][]model.PermissionID{
		AdminRole:    {accessperm.UsersManage, accessperm.RolesManage, accessperm.SparksManage, accessperm.EmbersManage, accessperm.ServersCreate, accessperm.ServersManageAll},
		CreatorRole:  {accessperm.ServersCreate},
		OwnerRole:    accessperm.All(),
		OperatorRole: accessperm.Legacy([]model.PermissionID{accessperm.ServerView, accessperm.ServerOperate}),
	}
	for roleID, actions := range permissions {
		for _, action := range actions {
			if err := tx.Create(&RolePermission{RoleID: roleID, Permission: action}).Error; err != nil {
				return err
			}
		}
	}

	if tx.Name() == "postgres" {
		return tx.Exec("SELECT setval(pg_get_serial_sequence('roles', 'id'), MAX(id)) FROM roles").Error
	}

	return nil
}
