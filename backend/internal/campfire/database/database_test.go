package database_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"campfire/internal/campfire/database"
)

func TestRejectNewerSchema(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "panel.db")

	db, err := database.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Exec("INSERT INTO schema_migrations(version) VALUES (5)").Error; err != nil {
		t.Fatal(err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := database.Open(ctx, path)
	if err == nil {
		_ = reopened.Close()

		t.Fatal("opened unsupported future schema")
	}

	if !strings.Contains(err.Error(), "newer than supported version 4") {
		t.Fatal("wrong schema version error", err)
	}
}

func TestUserProfileMigrationPreservesAccounts(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "panel.db")

	db, err := database.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}

	user := database.User{Username: "existing", PasswordHash: "preserved", Version: 4, RoleID: database.AdminRole}
	if err := db.ORM().Omit("Role").Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	// Recreate the deployed v1 shape before opening it with the new binary.
	for _, field := range []string{"CronExpression", "TimeZone", "OnlyWhenRunning"} {
		if err := db.ORM().Migrator().DropColumn(&database.Schedule{}, field); err != nil {
			t.Fatal(err)
		}
	}

	if err := db.ORM().Migrator().DropColumn(&database.ScheduleRun{}, "OnlyWhenRunning"); err != nil {
		t.Fatal(err)
	}

	for _, field := range []string{"Name", "Picture"} {
		if err := db.ORM().Migrator().DropColumn(&database.User{}, field); err != nil {
			t.Fatal(err)
		}
	}

	if err := db.ORM().Exec("UPDATE schema_migrations SET version = 1").Error; err != nil {
		t.Fatal(err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := database.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()

	var loaded database.User
	if err := reopened.ORM().Take(&loaded, user.ID).Error; err != nil {
		t.Fatal(err)
	}

	if loaded.Username != user.Username || loaded.PasswordHash != user.PasswordHash || loaded.Version != user.Version || loaded.RoleID != user.RoleID || loaded.Name != "" || loaded.Picture != "" {
		t.Fatal("profile migration changed existing account", loaded)
	}

	if err := reopened.ORM().Model(&loaded).Updates(map[string]any{"name": "Existing Admin", "picture": "https://example.com/avatar.png"}).Error; err != nil {
		t.Fatal("new profile columns are not writable", err)
	}
}

func TestUserEmailMigrationPreservesAccount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.db")

	db, err := database.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}

	user := database.User{Username: "existing-email", Name: "Existing User", PasswordHash: "preserved", Version: 7, RoleID: database.AdminRole}
	if err := db.ORM().Omit("Role").Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Migrator().DropColumn(&database.User{}, "Email"); err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Exec("UPDATE schema_migrations SET version = 3").Error; err != nil {
		t.Fatal(err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = database.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	var loaded database.User
	if err := db.ORM().Take(&loaded, user.ID).Error; err != nil {
		t.Fatal(err)
	}

	if loaded.Username != user.Username || loaded.Name != user.Name || loaded.PasswordHash != user.PasswordHash || loaded.Version != user.Version || loaded.Email != "" {
		t.Fatal("email migration changed account", loaded)
	}

	if err := db.ORM().Model(&loaded).Update("email", "existing@example.com").Error; err != nil {
		t.Fatal(err)
	}
}

func TestCronMigrationPreservesDevelopmentSchedules(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "panel.db")

	db, err := database.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}

	for _, item := range []any{
		&database.Spark{ID: "node", Name: "Node", Kind: "remote"},
		&database.Ember{ID: "ember", Name: "Game"},
		&database.Server{ID: "game", Name: "Game", SparkID: "node", EmberID: "ember"},
		&database.Schedule{ID: "existing", ServerID: "game", Name: "Restart", Action: "restart", CronExpression: "0 3 * * *", TimeZone: "UTC", Enabled: true, UserID: 1, Username: "admin", Version: 2, NextRunAt: 1},
	} {
		if err := db.ORM().Omit("Server", "Spark", "Ember").Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}

	for _, field := range []string{"CronExpression", "TimeZone", "OnlyWhenRunning"} {
		if err := db.ORM().Migrator().DropColumn(&database.Schedule{}, field); err != nil {
			t.Fatal(err)
		}
	}

	if err := db.ORM().Migrator().DropColumn(&database.ScheduleRun{}, "OnlyWhenRunning"); err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Exec("ALTER TABLE schedules ADD COLUMN interval_minutes INTEGER NOT NULL DEFAULT 1440").Error; err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Exec("UPDATE schema_migrations SET version = 2").Error; err != nil {
		t.Fatal(err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = database.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	var item database.Schedule
	if err := db.ORM().Take(&item, "id = ?", "existing").Error; err != nil {
		t.Fatal(err)
	}

	if item.Name != "Restart" || item.Action != "restart" || !item.Enabled || item.Version != 2 || item.CronExpression != "0 3 * * *" || item.TimeZone != "UTC" || item.OnlyWhenRunning || item.NextRunAt <= time.Now().Unix() {
		t.Fatal("migration damaged schedule", item)
	}

	if db.ORM().Migrator().HasColumn(&database.Schedule{}, "interval_minutes") {
		t.Fatal("interval column was retained")
	}
}

func TestInitialSchemaAndReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "panel.db")

	db, err := database.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	for _, table := range []any{
		&database.Spark{}, &database.Ember{}, &database.Server{}, &database.Role{},
		&database.RolePermission{}, &database.User{}, &database.UserSession{},
		&database.OIDCIdentity{}, &database.ServerOwnership{}, &database.ServerMember{},
		&database.SparkRole{}, &database.SparkEnrollment{}, &database.SparkIdentity{},
		&database.Schedule{}, &database.ScheduleRun{}, &database.AuditEvent{},
		&database.ServerTransfer{}, &database.BackupPolicy{}, &database.Backup{},
		&database.APIToken{}, &database.Notification{}, &database.NotificationRecipient{},
		&database.NotificationDelivery{}, &database.AlertState{},
	} {
		if !db.ORM().Migrator().HasTable(table) {
			t.Fatalf("missing initial table: %T", table)
		}
	}

	for _, table := range []string{"admin_credentials", "admin_sessions"} {
		if db.ORM().Migrator().HasTable(table) {
			t.Fatal("created retired authentication table", table)
		}
	}

	user := database.User{Username: "admin", PasswordHash: "preserved", Version: 4, RoleID: database.AdminRole}
	if err := db.ORM().Omit("Role").Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	for _, record := range []any{
		&database.OIDCIdentity{Issuer: "https://identity.example.com", Subject: "original", UserID: user.ID},
		&database.UserSession{Hash: "persisted", Provider: "https://identity.example.com", UserID: user.ID, CSRF: "csrf", CredentialVersion: 4, ExpiresAt: 9999999999, LastSeenAt: 1},
	} {
		if err := db.ORM().Omit("User").Create(record).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := database.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()

	var versions []int
	if err := reopened.ORM().Table("schema_migrations").Pluck("version", &versions).Error; err != nil || len(versions) != 1 || versions[0] != 4 {
		t.Fatal("expected exactly one baseline migration", versions, err)
	}

	var roles []database.Role
	if err := reopened.ORM().Preload("Permissions").Order("id").Find(&roles).Error; err != nil || len(roles) != 5 || roles[0].ID != database.AdminRole || len(roles[0].Permissions) != 6 || roles[4].ID != database.OperatorRole {
		t.Fatal("built-in roles or permissions changed on reopen", roles, err)
	}

	var session database.UserSession
	if err := reopened.ORM().Preload("User").Take(&session, "hash = ?", "persisted").Error; err != nil {
		t.Fatal(err)
	}

	if session.Provider != "https://identity.example.com" || session.User.PasswordHash != "preserved" || session.CredentialVersion != 4 {
		t.Fatal("reopen changed authentication")
	}

	var identity database.OIDCIdentity
	if err := reopened.ORM().Take(&identity, "subject = ?", "original").Error; err != nil || identity.UserID != user.ID {
		t.Fatal("reopen changed OIDC identity", err)
	}
}
