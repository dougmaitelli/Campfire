package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
)

type startupLogWriter func([]byte) (int, error)

func (write startupLogWriter) Write(data []byte) (int, error) { return write(data) }

func TestStartupLogsSetupTokenOnlyBeforeFirstAccount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.db")
	t.Setenv("CAMPFIRE_S3_BUCKET", "")

	previous := slog.Default()

	t.Cleanup(func() { slog.SetDefault(previous) })

	run := func() string {
		t.Helper()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var output bytes.Buffer

		writer := startupLogWriter(func(data []byte) (int, error) {
			n, err := output.Write(data)
			if bytes.Contains(data, []byte("Campfire listener started")) {
				cancel()
			}

			return n, err
		})
		slog.SetDefault(slog.New(slog.NewJSONHandler(writer, nil)))

		if err := Run(ctx, []string{"--database", path, "--listen", "127.0.0.1:0", "--public-url", "", "--agent-listen", "", "--oidc-issuer", "", "--oidc-client-id", "", "--oidc-client-secret", "", "--disable-local-login=false"}); err != nil {
			t.Fatal(err)
		}

		return output.String()
	}
	output := run()

	var token string

	for line := range strings.SplitSeq(strings.TrimSpace(output), "\n") {
		var event struct {
			Token string `json:"setup_token"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}

		if event.Token != "" {
			if token != "" {
				t.Fatal("setup token printed more than once")
			}

			token = event.Token
		}
	}

	if len(token) != 43 {
		t.Fatal("startup did not print a setup token")
	}

	if _, err := os.Stat(path + ".setup-token"); !os.IsNotExist(err) {
		t.Fatal("startup persisted a setup secret", err)
	}

	db, err := database.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}

	auth := service.NewAuthService(repository.NewAuthRepository(db.ORM()))
	_, createErr := auth.Access().CreateUser(context.Background(), "admin", "a long administrator test password", database.AdminRole, false, "", "")

	closeErr := db.Close()
	if createErr != nil || closeErr != nil {
		t.Fatal(createErr, closeErr)
	}

	if output := run(); strings.Contains(output, "setup_token") || strings.Contains(output, token) {
		t.Fatal("configured startup printed a setup secret")
	}
}

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
	if err := store.Setup(ctx, database.User{ID: 1, Username: "admin", PasswordHash: "old", Version: 1}, database.UserSession{Hash: "old-session", CSRF: "csrf", CredentialVersion: 1, ExpiresAt: time.Now().Add(time.Hour).Unix(), LastSeenAt: time.Now().Unix()}); err != nil {
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
