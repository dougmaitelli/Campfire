package repository_test

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"

	"gorm.io/gorm"
)

func TestUpgradeAddsOIDCLinkEmail(t *testing.T) {
	for _, version := range []int{11, 12} {
		t.Run(strconv.Itoa(version), func(t *testing.T) { testOIDCLinkUpgrade(t, version) })
	}
}

func testOIDCLinkUpgrade(t *testing.T, version int) {
	t.Helper()
	path := testDatabaseSource(t)

	db, err := database.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}

	user := database.User{Username: "existing", Email: "owner@example.com", PasswordHash: "preserved", CredentialVersion: 3, RoleID: database.UserRole}
	if err := db.ORM().Omit("Role").Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Omit("User").Create(&database.OIDCIdentity{Issuer: "issuer", Subject: "existing", UserID: user.ID}).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Migrator().DropIndex(&database.User{}, "users_oidc_link_email"); err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Exec("ALTER TABLE users DROP COLUMN oidc_link_email").Error; err != nil {
		t.Fatal(err)
	}

	if version == 12 {
		for _, statement := range []string{"CREATE TABLE email_tokens (hash TEXT PRIMARY KEY)", "CREATE TABLE verified_emails (user_id BIGINT PRIMARY KEY)"} {
			if err := db.ORM().Exec(statement).Error; err != nil {
				t.Fatal(err)
			}
		}
	}

	if err := db.ORM().Exec("UPDATE schema_migrations SET version = ?", version).Error; err != nil {
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

		var version int
		if err := db.ORM().Table("schema_migrations").Select("version").Scan(&version).Error; err != nil || version != 13 {
			t.Fatal("expected schema 13", version, err)
		}

		if db.ORM().Migrator().HasTable("email_tokens") || db.ORM().Migrator().HasTable("verified_emails") {
			t.Fatal("deprecated account-email tables survived upgrade")
		}

		var saved database.User
		if err := db.ORM().Take(&saved, user.ID).Error; err != nil || saved.OIDCLinkEmail != nil || saved.Email != user.Email || saved.PasswordHash != user.PasswordHash {
			t.Fatal("account changed", err)
		}

		if !saved.OIDCNameManaged || !saved.OIDCEmailManaged {
			t.Fatal("legacy OIDC link did not protect its profile")
		}

		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDuplicateLegacyOIDCSubjectsRequireExplicitSelection(t *testing.T) {
	path := testDatabaseSource(t)

	db, err := database.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}

	user := database.User{Username: "existing", RoleID: database.UserRole}
	if err := db.ORM().Omit("Role").Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Migrator().DropIndex(&database.OIDCIdentity{}, "oidc_user_issuer"); err != nil {
		t.Fatal(err)
	}

	links := []database.OIDCIdentity{{UserID: user.ID, Issuer: "provider", Subject: "keep"}, {UserID: user.ID, Issuer: "provider", Subject: "retire"}}
	if err := db.ORM().Omit("User").Create(&links).Error; err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	if _, err := database.Open(t.Context(), path); err == nil || !strings.Contains(err.Error(), "multiple OIDC subjects") {
		t.Fatal("ambiguous upgrade should identify duplicate links", err)
	}

	var count int64
	if err := db.ORM().Model(&database.OIDCIdentity{}).Count(&count).Error; err != nil || count != 2 {
		t.Fatal("upgrade changed ambiguous identities", count, err)
	}

	now := time.Now()
	store := repository.NewAuthRepository(db.ORM())
	session := database.UserSession{UserID: user.ID, Provider: "provider", CredentialVersion: user.CredentialVersion, Hash: "old-session", LastSeenAt: now.Unix(), ExpiresAt: now.Add(time.Hour).Unix()}
	token := database.APIToken{UserID: user.ID, CredentialVersion: user.CredentialVersion, Hash: "old-token", ExpiresAt: session.ExpiresAt}
	if err := store.CreateSession(t.Context(), session, "", now); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateAPIToken(t.Context(), &token, now); err != nil {
		t.Fatal(err)
	}
	if err := database.Transaction(db.ORM(), func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ? AND issuer = ? AND subject = ?", user.ID, links[1].Issuer, links[1].Subject).Delete(&database.OIDCIdentity{}).Error; err != nil {
			return err
		}
		return tx.Model(&user).Update("credential_version", gorm.Expr("credential_version + 1")).Error
	}); err != nil {
		t.Fatal(err)
	}

	upgraded, err := database.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = upgraded.Close() }()

	store = repository.NewAuthRepository(upgraded.ORM())
	if _, _, err := store.Session(t.Context(), session.Hash, now); !errors.Is(err, repository.ErrAuthentication) {
		t.Fatal("session survived duplicate identity repair", err)
	}
	if _, err := store.APIToken(t.Context(), token.Hash, now); !errors.Is(err, repository.ErrAuthentication) {
		t.Fatal("API token survived duplicate identity repair", err)
	}
	if err := upgraded.ORM().Omit("User").Create(&links[1]).Error; err == nil {
		t.Fatal("upgrade allowed a second subject for the same user and issuer")
	}
}
