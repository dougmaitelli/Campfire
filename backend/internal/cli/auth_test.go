package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"campfire/internal/database"
	"campfire/internal/database/repository"
	"campfire/internal/service"
)

func TestAdminOriginRequiresExplicitHTTPSForPublicAccess(t *testing.T) {
	for _, input := range []struct {
		listen string
		url    string
		valid  bool
	}{
		{"127.0.0.1:8080", "", true},
		{"[::1]:8080", "", true},
		{":8080", "", false},
		{"0.0.0.0:8080", "http://panel.example.com", false},
		{"127.0.0.1:8080", "http://127.0.0.1:8081", true},
		{"0.0.0.0:8080", "https://panel.example.com", true},
		{"127.0.0.1:8080", "https://user:password@panel.example.com", false},
		{"127.0.0.1:8080", "https://panel.example.com/path", false},
		{"127.0.0.1:8080", "https://panel.example.com?query=1", false},
		{"127.0.0.1:8080", "https://panel.example.com#fragment", false},
	} {
		_, err := adminOrigin(input.listen, input.url)
		if (err == nil) != input.valid {
			t.Errorf("listen %q public URL %q: %v", input.listen, input.url, err)
		}
	}

	if origin, err := adminOrigin("127.0.0.1:8080", "https://Panel.Example.com:443/"); err != nil || origin != "https://panel.example.com" {
		t.Fatal("public origin did not match browser normalization", origin, err)
	}
}

func TestResetPasswordCommandRevokesSessions(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "panel.db")

	db, err := database.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = db.Close() }()

	store := repository.NewAuthRepository(db.ORM())
	if err := store.Setup(ctx, database.AdminCredential{ID: 1, Username: "admin", PasswordHash: "old", Version: 1}, database.AdminSession{Hash: "old-session", CSRF: "csrf", CredentialVersion: 1, ExpiresAt: time.Now().Add(time.Hour).Unix(), LastSeenAt: time.Now().Unix()}); err != nil {
		t.Fatal(err)
	}

	password := "a replacement passphrase with spaces"

	file := filepath.Join(t.TempDir(), "password")
	if err := os.WriteFile(file, []byte(password+"\r\n"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := Run(ctx, []string{"reset-password", "--database", path, "--password-file", file}); err != nil {
		t.Fatal(err)
	}

	if _, _, err := service.NewAuthService(store).Login(ctx, "admin", password, ""); err != nil {
		t.Fatal("replacement password did not work", err)
	}

	if _, _, err := store.Session(ctx, "old-session", time.Now()); err == nil {
		t.Fatal("reset command left a prior session valid")
	}

	if err := os.Chmod(file, 0644); err != nil {
		t.Fatal(err)
	}

	if err := Run(ctx, []string{"reset-password", "--database", path, "--password-file", file}); err == nil {
		t.Fatal("public password file accepted")
	}

	if err := Run(ctx, []string{"reset-password", "--database", path}); err == nil {
		t.Fatal("missing password file accepted")
	}
}
