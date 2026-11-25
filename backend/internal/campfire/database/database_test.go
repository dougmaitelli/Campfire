package database_test

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"campfire/internal/campfire/database"
)

func TestRejectUnsupportedSchemaWithoutChangingData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.db")

	db, err := database.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Exec("UPDATE schema_migrations SET version = 999").Error; err != nil {
		t.Fatal(err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := database.Open(t.Context(), path)
	if err == nil {
		_ = reopened.Close()

		t.Fatal("opened unsupported schema")
	}

	if !strings.Contains(err.Error(), "unsupported database schema history") {
		t.Fatal(err)
	}
}

func TestIncompleteBaselineIsNotRepaired(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.db")

	db, err := database.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Migrator().DropTable(&database.GameIntegrationRegistration{}); err != nil {
		t.Fatal(err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := database.Open(t.Context(), path)
	if err == nil {
		_ = reopened.Close()

		t.Fatal("silently repaired an incomplete baseline")
	}

	if !strings.Contains(err.Error(), "missing table game_integration_registrations") {
		t.Fatal(err)
	}
}

func TestBaselineReopenPreservesData(t *testing.T) {
	// Marker 2 identifies the same physical schema, not an older schema to migrate.
	// Both paths must reopen without changing account or session data.
	for _, marker := range []int{1, 2, 3, 4, 5, 6} {
		t.Run(strconv.Itoa(marker), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "panel.db")

			db, err := database.Open(t.Context(), path)
			if err != nil {
				t.Fatal(err)
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

			var initial []int
			if err := db.ORM().Table("schema_migrations").Pluck("version", &initial).Error; err != nil || len(initial) != 1 || initial[0] != 12 {
				t.Fatal("expected schema 12", initial, err)
			}

			if marker < 5 {
				if err := db.ORM().Migrator().DropTable(&database.ServerCrash{}); err != nil {
					t.Fatal(err)
				}
			}

			if err := db.ORM().Exec("UPDATE schema_migrations SET version = ?", marker).Error; err != nil {
				t.Fatal(err)
			}

			if err := db.Close(); err != nil {
				t.Fatal(err)
			}

			for range 2 {
				reopened, err := database.Open(t.Context(), path)
				if err != nil {
					t.Fatal(err)
				}

				if !reopened.ORM().Migrator().HasTable(&database.ServerCrash{}) {
					t.Fatal("crash history migration missing")
				}

				var versions []int
				if err := reopened.ORM().Table("schema_migrations").Pluck("version", &versions).Error; err != nil || len(versions) != 1 || versions[0] != 12 {
					t.Fatal("incorrect baseline marker", versions, err)
				}

				var roles []database.Role
				if err := reopened.ORM().Preload("Permissions").Order("id").Find(&roles).Error; err != nil || len(roles) != 5 || roles[0].ID != database.AdminRole || len(roles[0].Permissions) != 6 || roles[4].ID != database.OperatorRole {
					t.Fatal("roles changed", roles, err)
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
					t.Fatal("reopen changed identity", err)
				}

				if err := reopened.Close(); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
