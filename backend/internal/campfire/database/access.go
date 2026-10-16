package database

import "gorm.io/gorm"

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
		{ID: AdminRole, Name: "Admin", Scope: "platform", Builtin: true},
		{ID: CreatorRole, Name: "Server creator", Scope: "platform", Builtin: true},
		{ID: UserRole, Name: "User", Scope: "platform", Builtin: true},
		{ID: OwnerRole, Name: "Owner", Scope: "server", Builtin: true},
		{ID: OperatorRole, Name: "Operator", Scope: "server", Builtin: true},
	}
	for _, role := range roles {
		if err := tx.Create(&role).Error; err != nil {
			return err
		}
	}

	permissions := map[int64][]string{
		AdminRole:    {"users.manage", "roles.manage", "sparks.manage", "embers.manage", "servers.create", "servers.manage_all"},
		CreatorRole:  {"servers.create"},
		OwnerRole:    {"server.view", "server.operate", "server.configure", "server.delete", "server.share"},
		OperatorRole: {"server.view", "server.operate"},
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
