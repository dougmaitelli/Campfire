package repository_test

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
)

func TestDeletedUserRetainsAuditProfileAndRevokesAccess(t *testing.T) {
	db, err := database.Open(t.Context(), filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	user := database.User{Username: "former", Name: "Former User", Email: "former@example.test", PasswordHash: "encoded", CredentialVersion: 3, RoleID: database.UserRole}
	if err := db.ORM().Omit("Role").Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	for _, record := range []any{
		&database.UserSession{Hash: "session", UserID: user.ID, CSRF: "csrf", CredentialVersion: 3, ExpiresAt: now.Add(time.Hour).Unix(), LastSeenAt: now.Unix()},
		&database.OIDCIdentity{Issuer: "issuer", Subject: "subject", UserID: user.ID},
	} {
		if err := db.ORM().Omit("User").Create(record).Error; err != nil {
			t.Fatal(err)
		}
	}

	auth := repository.NewAuthRepository(db.ORM())

	token := database.APIToken{UserID: user.ID, Hash: "token", CredentialVersion: 3, ExpiresAt: now.Add(time.Hour).Unix()}
	if err := auth.CreateAPIToken(t.Context(), &token, now); err != nil {
		t.Fatal(err)
	}

	activity := repository.NewActivityRepository(db.ORM())

	event := database.AuditEvent{ID: "event", UserID: &user.ID, Username: "name-at-the-time", Action: "test", CreatedAt: now.Unix()}
	if err := activity.BeginAudit(t.Context(), &event); err != nil {
		t.Fatal(err)
	}

	if err := auth.Access().DeleteUser(t.Context(), user.ID); err != nil {
		t.Fatal(err)
	}

	users, err := auth.Access().Users(t.Context())
	if err != nil || len(users) != 0 {
		t.Fatal("deleted user remained visible", users, err)
	}

	credential, err := auth.FindCredential(t.Context(), user.Username)
	if err != nil || credential != nil {
		t.Fatal("deleted user can authenticate", credential, err)
	}

	if _, _, err := auth.Session(t.Context(), "session", now); !errors.Is(err, repository.ErrAuthentication) {
		t.Fatal("session survived deletion", err)
	}

	if _, err := auth.APIToken(t.Context(), "token", now); !errors.Is(err, repository.ErrAuthentication) {
		t.Fatal("API token survived deletion", err)
	}

	if _, err := auth.OIDCAccount(t.Context(), "issuer", "subject", "newname", 0, 0, "", "", "", ""); err == nil {
		t.Fatal("deleted OIDC identity was reprovisioned")
	}

	items, err := activity.Audit(t.Context(), "", "", "")
	if err != nil || len(items) != 1 {
		t.Fatal(items, err)
	}

	actor := items[0].User
	if actor == nil || !actor.DeletedAt.Valid || actor.Email != user.Email || actor.Name != user.Name || items[0].Username != event.Username || actor.PasswordHash != "" {
		t.Fatal("lost retained profile or historical identity", items[0])
	}

	if err := db.ORM().Unscoped().Delete(&database.User{}, user.ID).Error; err == nil {
		t.Fatal("audit FK did not protect retained user")
	}

	if err := auth.Access().CreateUser(t.Context(), &database.User{Username: user.Username, RoleID: database.UserRole}); !errors.Is(err, repository.ErrAccessConflict) {
		t.Fatal("reused deleted account's username", err)
	}
}
