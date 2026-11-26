package database_test

import (
	"path/filepath"
	"testing"

	"campfire/internal/campfire/database"
)

func TestSchemaTenPreservesExistingAuthenticator(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mfa-v10.db")

	db, err := database.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}

	user := database.User{Username: "existing", PasswordHash: "preserved", CredentialVersion: 4, RoleID: database.UserRole}
	if err := db.ORM().Omit("Role").Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	factor := database.UserMFA{UserID: user.ID, SecretRef: "encrypted-reference", LastCounter: 123, RecoveryHashes: []string{"hash"}}
	if err := db.ORM().Omit("User").Create(&factor).Error; err != nil {
		t.Fatal(err)
	}

	for _, field := range []string{"Devices", "PendingName"} {
		if err := db.ORM().Migrator().DropColumn(&database.UserMFA{}, field); err != nil {
			t.Fatal(err)
		}
	}

	if err := db.ORM().Exec("UPDATE schema_migrations SET version = 10").Error; err != nil {
		t.Fatal(err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	for range 2 {
		db, err = database.Open(t.Context(), path)
		if err != nil {
			t.Fatal(err)
		}

		var migrated database.UserMFA
		if err := db.ORM().Take(&migrated, "user_id = ?", user.ID).Error; err != nil {
			t.Fatal(err)
		}

		if len(migrated.Devices) != 1 || migrated.Devices[0].SecretRef != factor.SecretRef || migrated.Devices[0].LastCounter != factor.LastCounter || len(migrated.RecoveryHashes) != 1 {
			t.Fatal("lost existing authenticator", migrated)
		}

		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSchemaNineAddsMFAWithoutChangingAccounts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mfa.db")

	db, err := database.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}

	user := database.User{Username: "existing", PasswordHash: "preserved", CredentialVersion: 4, RoleID: database.UserRole}
	if err := db.ORM().Omit("Role").Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Migrator().DropTable(&database.UserMFA{}); err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Exec("UPDATE schema_migrations SET version = 9").Error; err != nil {
		t.Fatal(err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	for range 2 {
		db, err = database.Open(t.Context(), path)
		if err != nil {
			t.Fatal(err)
		}

		if !db.ORM().Migrator().HasTable(&database.UserMFA{}) {
			t.Fatal("MFA table missing")
		}

		var saved database.User
		if err := db.ORM().Take(&saved, user.ID).Error; err != nil || saved.PasswordHash != user.PasswordHash || saved.CredentialVersion != user.CredentialVersion {
			t.Fatal("migration altered account", err)
		}

		var count int64
		if err := db.ORM().Model(&database.UserMFA{}).Count(&count).Error; err != nil || count != 0 {
			t.Fatal("MFA unexpectedly enabled", count, err)
		}

		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
