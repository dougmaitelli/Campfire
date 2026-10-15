package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"campfire/internal/database"
	"campfire/internal/database/repository"
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

	path := filepath.Join(directory, "setup-token")
	if err := auth.PrepareSetup(ctx, path); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("bootstrap token was not private", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// A restart must preserve the same bootstrap token until successful setup.
	restarted := NewAuthService(store)
	if err := restarted.PrepareSetup(ctx, path); err != nil {
		t.Fatal(err)
	}

	password := "a private long passphrase with spaces"

	token := strings.TrimSpace(string(data))
	if _, _, err := restarted.Setup(ctx, token, "admin", "short"); !errors.Is(err, ErrAuthInput) {
		t.Fatal("short setup password accepted", err)
	}

	session, raw, err := restarted.Setup(ctx, token, " AdMin ", password)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("consumed setup file retained")
	}

	admin, err := store.Credential(ctx)
	if err != nil || admin.Username != "admin" || strings.Contains(admin.PasswordHash, password) || !strings.HasPrefix(admin.PasswordHash, "$pbkdf2-sha256$600000$") {
		t.Fatal("password was not safely encoded", err)
	}

	var stored database.AdminSession
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

func TestSetupRejectsPublicOrLinkedBootstrapFiles(t *testing.T) {
	ctx := context.Background()

	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = db.Close() }()

	path := filepath.Join(t.TempDir(), "setup-token")
	if err := os.WriteFile(path, []byte(authToken()), 0644); err != nil {
		t.Fatal(err)
	}

	auth := NewAuthService(repository.NewAuthRepository(db.ORM()))
	if err := auth.PrepareSetup(ctx, path); err == nil {
		t.Fatal("public setup token accepted")
	}

	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}

	if err := auth.PrepareSetup(ctx, link); err == nil {
		t.Fatal("bootstrap symlink accepted")
	}
}
