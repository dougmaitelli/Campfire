package service

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
)

func TestAdministratorSecretsBootstrapAndThrottling(t *testing.T) {
	ctx := context.Background()
	directory := t.TempDir()

	db, err := database.Open(ctx, filepath.Join(directory, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = db.Close() }()

	store := repository.NewAuthRepository(db.ORM())
	auth := NewAuthService(store)

	previous, err := auth.PrepareSetup(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if !validAuthToken(previous) {
		t.Fatal("invalid setup token")
	}

	// Restarting generates a new console secret rather than recovering a file.
	restarted := NewAuthService(store)

	token, err := restarted.PrepareSetup(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if token == previous || !validAuthToken(token) {
		t.Fatal("restart did not replace setup token")
	}

	password := "a private long passphrase with spaces"

	if _, _, err := restarted.Setup(ctx, previous, "admin", password); !errors.Is(err, ErrAuthentication) {
		t.Fatal("old process token accepted", err)
	}

	if _, _, err := restarted.Setup(ctx, token, "admin", "short"); !errors.Is(err, ErrAuthInput) {
		t.Fatal("short setup password accepted", err)
	}

	session, raw, err := restarted.Setup(ctx, token, " AdMin ", password)
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := restarted.Setup(ctx, token, "other", password); !errors.Is(err, ErrAuthentication) {
		t.Fatal("consumed token was reusable", err)
	}

	if secret, err := NewAuthService(store).PrepareSetup(ctx); err != nil || secret != "" {
		t.Fatal("configured installation generated another setup token", err)
	}

	admin, err := store.Credential(ctx)
	if err != nil || admin.Username != "admin" || strings.Contains(admin.PasswordHash, password) || !strings.HasPrefix(admin.PasswordHash, "$pbkdf2-sha256$600000$") {
		t.Fatal("password was not safely encoded", err)
	}

	var stored database.UserSession
	if err := db.ORM().Take(&stored).Error; err != nil || stored.Hash == raw || stored.Hash != tokenHash(raw) || session.CSRF != stored.CSRF {
		t.Fatal("session secret persisted without hashing", err)
	}

	if err := restarted.ResetPassword(ctx, "short"); !errors.Is(err, ErrAuthInput) {
		t.Fatal("short reset password accepted", err)
	}

	if _, _, err := restarted.Session(ctx, raw); err != nil {
		t.Fatal("invalid reset revoked a valid session", err)
	}

	now := time.Now()
	restarted.now = func() time.Time { return now }

	for range 10 {
		if !restarted.AllowAttempt("client") {
			t.Fatal("throttled before limit")
		}
	}

	if restarted.AllowAttempt("client") {
		t.Fatal("attempt limit not enforced")
	}

	now = now.Add(time.Minute)

	if !restarted.AllowAttempt("client") {
		t.Fatal("attempt window never resets")
	}
}

func TestSetupRequiresAnExplicitlyPreparedSecret(t *testing.T) {
	ctx := context.Background()

	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = db.Close() }()

	auth := NewAuthService(repository.NewAuthRepository(db.ORM()))
	if _, _, err := auth.Setup(ctx, authToken(), "admin", "a private long passphrase"); !errors.Is(err, ErrAuthentication) {
		t.Fatal("setup accepted before preparation", err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	if token, err := auth.PrepareSetup(ctx); err == nil || token != "" {
		t.Fatal("setup secret generated when account state could not be checked", err)
	}
}
