package database_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"campfire/internal/campfire/database"
)

func TestRejectNewerSchema(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "panel.db")

	db, err := database.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Exec("INSERT INTO schema_migrations(version) VALUES (3)").Error; err != nil {
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

	if !strings.Contains(err.Error(), "newer than supported version 2") {
		t.Fatal("wrong schema version error", err)
	}
}

func TestRejectPreBaselineSchema(t *testing.T) {
	for _, version := range []int{1, 2, 3, 4} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "panel.db")

			db, err := database.Open(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}

			if err := db.ORM().Migrator().RenameColumn(&database.User{}, "credential_version", "version"); err != nil {
				t.Fatal(err)
			}

			if err := db.ORM().Exec("UPDATE schema_migrations SET version = ?", version).Error; err != nil {
				t.Fatal(err)
			}

			if err := db.Close(); err != nil {
				t.Fatal(err)
			}

			reopened, err := database.Open(t.Context(), path)
			if err == nil {
				_ = reopened.Close()

				t.Fatal("opened pre-baseline schema")
			}

			if !strings.Contains(err.Error(), "upgrade to Campfire commit 204d134 first") {
				t.Fatal("missing upgrade instructions", err)
			}
		})
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
		&database.OIDCIdentity{}, &database.ServerAccessGrant{},
		&database.SparkAccessGrant{}, &database.SparkIdentity{},
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

	if db.ORM().Migrator().HasColumn(&database.Ember{}, "update_checked_url") {
		t.Fatal("duplicated the Ember definition's update URL")
	}

	user := database.User{Username: "admin", PasswordHash: "preserved", CredentialVersion: 4, RoleID: database.AdminRole}
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

	var initialVersions []int
	if err := db.ORM().Table("schema_migrations").Pluck("version", &initialVersions).Error; err != nil || len(initialVersions) != 1 || initialVersions[0] != 2 {
		t.Fatal("expected one baseline migration", initialVersions, err)
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
	if err := reopened.ORM().Table("schema_migrations").Order("version").Pluck("version", &versions).Error; err != nil || len(versions) != 1 || versions[0] != 2 {
		t.Fatal("upgraded migration history changed", versions, err)
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
