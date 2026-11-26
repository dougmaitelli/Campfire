package repository_test

import (
	"path/filepath"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/limits"
)

func TestSessionListingFiltersAndOwnerScopedRevocation(t *testing.T) {
	ctx := t.Context()

	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "sessions.db"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	now := time.Now()
	store := repository.NewAuthRepository(db.ORM())

	current := database.UserSession{Hash: "current", UserID: 1, CredentialVersion: 1, CSRF: "csrf", ExpiresAt: now.Add(time.Hour).Unix(), LastSeenAt: now.Unix()}
	if err := store.Setup(ctx, database.User{ID: 1, Username: "admin", CredentialVersion: 1}, current); err != nil {
		t.Fatal(err)
	}

	other := database.User{ID: 2, Username: "other", CredentialVersion: 1, RoleID: database.UserRole}
	if err := db.ORM().Omit("Role").Create(&other).Error; err != nil {
		t.Fatal(err)
	}

	for _, kind := range []string{"active", "expired", "idle", "old-credentials", "other"} {
		session := current

		session.Hash = kind
		switch kind {
		case "expired":
			session.ExpiresAt = now.Unix()
		case "idle":
			session.LastSeenAt = now.Add(-limits.SessionIdleTimeout).Unix()
		case "old-credentials":
			session.CredentialVersion = 0
		case "other":
			session.UserID = other.ID
		}

		if err := db.ORM().Omit("User").Create(&session).Error; err != nil {
			t.Fatal(err)
		}
	}

	sessions, err := store.Sessions(ctx, current, now)
	if err != nil || len(sessions) != 2 {
		t.Fatal(sessions, err)
	}

	if err := store.RevokeOwnedSession(ctx, other.ID, current.Hash); err != nil {
		t.Fatal(err)
	}

	if _, _, err := store.Session(ctx, current.Hash, now); err != nil {
		t.Fatal("cross-account revoke", err)
	}

	if err := store.RevokeOwnedSession(ctx, current.UserID, "active"); err != nil {
		t.Fatal(err)
	}

	if _, _, err := store.Session(ctx, "active", now); err == nil {
		t.Fatal("revoked session still valid")
	}

	if err := store.RevokeOtherSessions(ctx, current); err != nil {
		t.Fatal(err)
	}

	for _, hash := range []string{current.Hash, "other"} {
		if _, _, err := store.Session(ctx, hash, now); err != nil {
			t.Fatal("bulk revoke affected protected session", hash, err)
		}
	}

	var count int64
	if err := db.ORM().Model(&database.UserSession{}).Count(&count).Error; err != nil || count != 2 {
		t.Fatal(count, err)
	}
}
