package database_test

import (
	"context"
	"path/filepath"
	"testing"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"
)

func TestRejectNewerSchema(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "panel.db")

	db, err := database.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().WithContext(ctx).Exec("INSERT INTO schema_migrations(version) VALUES (9)").Error; err != nil {
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
}

func TestOIDCMigrationPreservesAccountsAndLocalSessions(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "panel.db")

	db, err := database.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Omit("Role").Create(&database.User{ID: 1, Username: "admin", PasswordHash: "preserved", Version: 3, RoleID: database.AdminRole}).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Omit("User").Create(&database.UserSession{Hash: "session", UserID: 1, CSRF: "csrf", CredentialVersion: 3, ExpiresAt: 9999999999, LastSeenAt: 1}).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Migrator().DropTable(&database.OIDCIdentity{}); err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Migrator().DropColumn(&database.UserSession{}, "Provider"); err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Exec("DELETE FROM schema_migrations WHERE version = 8").Error; err != nil {
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

	var session database.UserSession
	if err := reopened.ORM().Preload("User").Take(&session, "hash = ?", "session").Error; err != nil {
		t.Fatal(err)
	}

	if session.Provider != "" || session.User.PasswordHash != "preserved" || session.User.Version != 3 || session.CSRF != "csrf" || !reopened.ORM().Migrator().HasTable(&database.OIDCIdentity{}) {
		t.Fatal("OIDC migration changed existing authentication")
	}
}

func TestAuthenticationMigrationPreservesPopulatedVersionFive(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "panel.db")

	db, err := database.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	for _, record := range []any{
		&database.Spark{ID: "node", Name: "Preserved node", Kind: "remote", CreatedAt: "2026-10-14T00:00:00Z"},
		&database.Ember{ID: "ember", Name: "Preserved ember", Definition: model.Ember{Schema: 1, Name: "Preserved ember"}, CreatedAt: "2026-10-14T00:00:00Z"},
		&database.Server{ID: "server", Name: "Preserved server", SparkID: "node", EmberID: "ember", Image: "alpine:3.22", Variables: map[string]string{"KEEP": "value"}, CPULimit: 1, MemoryMB: 128, DiskMB: 1024, Ports: []model.ServerPort{{BindIP: "127.0.0.1", HostPort: 25565, ContainerPort: 25565, Protocol: "tcp"}}, CreatedAt: "2026-10-14T00:00:00Z"},
		&database.SparkIdentity{SparkID: "node", CertificateHash: "preserved-certificate"},
	} {
		if err := db.ORM().Create(record).Error; err != nil {
			t.Fatal(err)
		}
	}

	// Recreate the pre-auth schema by removing only the new tables and migration.
	if err := db.ORM().Migrator().DropTable(&database.OIDCIdentity{}, &database.ServerMember{}, &database.ServerOwnership{}, &database.SparkRole{}, &database.UserSession{}, &database.User{}, &database.RolePermission{}, &database.Role{}); err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Exec("DELETE FROM schema_migrations WHERE version >= 6").Error; err != nil {
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

	var server database.Server
	if err := reopened.ORM().Preload("Spark").Preload("Ember").Take(&server, "id = ?", "server").Error; err != nil {
		t.Fatal(err)
	}

	if server.Name != "Preserved server" || server.Variables["KEEP"] != "value" || server.Spark.Name != "Preserved node" || server.Ember.Definition.Name != "Preserved ember" || len(server.Ports) != 1 || server.Ports[0].HostPort != 25565 {
		t.Fatal("authentication migration changed existing game configuration")
	}

	var identity database.SparkIdentity
	if err := reopened.ORM().Take(&identity, "spark_id = ?", "node").Error; err != nil || identity.CertificateHash != "preserved-certificate" {
		t.Fatal("authentication migration changed node identity", err)
	}

	if !reopened.ORM().Migrator().HasTable(&database.User{}) || !reopened.ORM().Migrator().HasTable(&database.UserSession{}) {
		t.Fatal("authentication tables were not created")
	}
}

func TestMultiUserMigrationPreservesAdministratorAndClaimsServers(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "panel.db")

	db, err := database.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = db.Close() }()

	for _, record := range []any{
		&database.Spark{ID: "node", Name: "Node", Kind: "remote", CreatedAt: "2026-10-14T00:00:00Z"},
		&database.Ember{ID: "ember", Name: "Game", Definition: model.Ember{Schema: 1, Name: "Game"}, CreatedAt: "2026-10-14T00:00:00Z"},
		&database.Server{ID: "game", Name: "Game", SparkID: "node", EmberID: "ember", Image: "alpine:3.22", Variables: map[string]string{}, Ports: []model.ServerPort{}, CreatedAt: "2026-10-14T00:00:00Z"},
	} {
		if err := db.ORM().Create(record).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := db.ORM().Migrator().DropTable(&database.OIDCIdentity{}, &database.ServerMember{}, &database.ServerOwnership{}, &database.SparkRole{}, &database.UserSession{}, &database.User{}, &database.RolePermission{}, &database.Role{}); err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Migrator().CreateTable(&database.AdminCredential{}, &database.AdminSession{}); err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Create(&database.AdminCredential{ID: 1, Username: "original", PasswordHash: "unchanged-password-hash", Version: 9}).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Create(&database.AdminSession{Hash: "old-singleton-session", CSRF: "old-csrf", CredentialVersion: 9, ExpiresAt: 9999999999}).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Exec("DELETE FROM schema_migrations WHERE version >= 7").Error; err != nil {
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

	var user database.User
	if err := reopened.ORM().Take(&user, 1).Error; err != nil {
		t.Fatal(err)
	}

	if user.Username != "original" || user.PasswordHash != "unchanged-password-hash" || user.Version != 9 || user.RoleID != database.AdminRole {
		t.Fatal("administrator identity changed", user.Username)
	}

	var ownership database.ServerOwnership
	if err := reopened.ORM().Take(&ownership, "server_id = ?", "game").Error; err != nil || ownership.UserID != user.ID {
		t.Fatal("existing server did not retain administrator ownership", err)
	}

	var sessions int64
	if err := reopened.ORM().Model(&database.UserSession{}).Count(&sessions).Error; err != nil || sessions != 0 {
		t.Fatal("unbound sessions migrated", err)
	}

	if reopened.ORM().Migrator().HasTable(&database.AdminCredential{}) || reopened.ORM().Migrator().HasTable(&database.AdminSession{}) {
		t.Fatal("obsolete singleton authentication tables remain")
	}
}
