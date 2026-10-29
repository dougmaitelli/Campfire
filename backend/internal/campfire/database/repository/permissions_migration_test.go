package repository_test

import (
	"slices"
	"testing"

	"campfire/internal/campfire/database"
	accessperm "campfire/internal/shared/permissions"
)

func TestFinePermissionsMigrationPreservesLegacyAccessAndRunsOnce(t *testing.T) {
	source := testDatabaseSource(t)

	db, err := database.Open(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}

	role := database.Role{Name: "Legacy custom role", Scope: "server"}
	if err := db.ORM().Create(&role).Error; err != nil {
		t.Fatal(err)
	}

	for _, action := range []string{accessperm.ServerView, accessperm.ServerOperate, accessperm.ServerConfigure, accessperm.LegacyServerShare} {
		if err := db.ORM().Create(&database.RolePermission{RoleID: role.ID, Permission: action}).Error; err != nil {
			t.Fatal(err)
		}
	}

	user := database.User{Username: "legacy-user", PasswordHash: "preserved", CredentialVersion: 1, RoleID: database.UserRole}
	if err := db.ORM().Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	token := database.APIToken{UserID: user.ID, CredentialVersion: 1, Name: "Legacy", Hash: "preserved-token-hash", Prefix: "cf_legacy", Scopes: []string{accessperm.ServerView, accessperm.ServerOperate}}
	if err := db.ORM().Create(&token).Error; err != nil {
		t.Fatal(err)
	}

	for _, oldColumn := range []struct {
		model any
		name  string
	}{
		{&database.Schedule{}, "AuthorizationScopes"},
		{&database.ScheduleRun{}, "AuthorizationScopes"},
		{&database.ScheduleRun{}, "AuthorizationPermission"},
	} {
		if err := db.ORM().Migrator().DropColumn(oldColumn.model, oldColumn.name); err != nil {
			t.Fatal(err)
		}
	}

	if err := db.ORM().Exec("UPDATE schema_migrations SET version = 1").Error; err != nil {
		t.Fatal(err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = database.Open(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Preload("Permissions").Take(&role, role.ID).Error; err != nil {
		t.Fatal(err)
	}

	actions := make([]string, 0, len(role.Permissions))
	for _, action := range role.Permissions {
		actions = append(actions, action.Permission)
	}

	for _, action := range []string{accessperm.ServerFilesWrite, accessperm.ServerFilesDelete, accessperm.ServerConsoleWrite, accessperm.ServerBackupsRestore, accessperm.ServerReinstall, accessperm.ServerSchedulesManage} {
		if !slices.Contains(actions, action) {
			t.Fatal("legacy role lost access", action, actions)
		}
	}

	if slices.Contains(actions, accessperm.LegacyServerShare) {
		t.Fatal("legacy delegation survived")
	}

	if err := db.ORM().Take(&token, token.ID).Error; err != nil {
		t.Fatal(err)
	}

	if token.Hash != "preserved-token-hash" || !slices.Contains(token.Scopes, accessperm.ServerFilesWrite) || slices.Contains(token.Scopes, accessperm.LegacyServerShare) {
		t.Fatal("token migration changed credentials or lost scope", token.Scopes)
	}

	if err := db.ORM().Where("role_id = ?", role.ID).Delete(&database.RolePermission{}).Error; err != nil {
		t.Fatal(err)
	}

	for _, action := range accessperm.Expand([]string{accessperm.ServerOperate}) {
		if err := db.ORM().Create(&database.RolePermission{RoleID: role.ID, Permission: action}).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = database.Open(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	role.Permissions = nil
	if err := db.ORM().Preload("Permissions").Take(&role, role.ID).Error; err != nil {
		t.Fatal(err)
	}

	for _, action := range role.Permissions {
		if action.Permission == accessperm.ServerFilesWrite {
			t.Fatal("legacy migration ran again")
		}
	}
}
