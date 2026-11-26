package database_test

import (
	"path/filepath"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
)

func TestSessionMetadataUpgradePreservesExistingLogin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.db")

	db, err := database.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	now := time.Now()

	session := database.UserSession{Hash: "existing-session", UserID: 1, CSRF: "existing-csrf", CredentialVersion: 1, ExpiresAt: now.Add(time.Hour).Unix(), LastSeenAt: now.Unix()}
	if err := repository.NewAuthRepository(db.ORM()).Setup(t.Context(), database.User{ID: 1, Username: "admin", CredentialVersion: 1}, session); err != nil {
		t.Fatal(err)
	}
	// Reconstruct the version-8 session schema, rather than only changing its marker.
	for _, query := range []string{
		"ALTER TABLE user_sessions DROP COLUMN created_at",
		"ALTER TABLE user_sessions DROP COLUMN user_agent",
		"UPDATE schema_migrations SET version = 8",
	} {
		if err := db.ORM().Exec(query).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	// Reopening twice also checks that the upgrade is idempotent.
	for range 2 {
		reopened, err := database.Open(t.Context(), path)
		if err != nil {
			t.Fatal(err)
		}

		t.Cleanup(func() { _ = reopened.Close() })

		var version int
		if err := reopened.ORM().Table("schema_migrations").Select("version").Scan(&version).Error; err != nil || version != 13 {
			t.Fatal(version, err)
		}

		stored, username, err := repository.NewAuthRepository(reopened.ORM()).Session(t.Context(), session.Hash, now)
		if err != nil {
			t.Fatal("existing login was invalidated", err)
		}

		if username != "admin" || stored.CSRF != session.CSRF || stored.ExpiresAt != session.ExpiresAt || stored.UserID != session.UserID || stored.CredentialVersion != session.CredentialVersion {
			t.Fatal("session changed during upgrade", stored)
		}

		if stored.CreatedAt != 0 || stored.UserAgent != "" {
			t.Fatal("legacy metadata must remain unknown", stored)
		}

		if err := reopened.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
