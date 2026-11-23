package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/settings"
)

type startupLogWriter func([]byte) (int, error)

func TestRecoveryPasswordPrompt(t *testing.T) {
	for _, tc := range []struct {
		name, first, second string
		readError           bool
		wantError           bool
	}{
		{"matching", " My new password 1! ", " My new password 1! ", false, false},
		{"mismatch", "My new password 1!", "Different password 1!", false, true},
		{"read error", "", "", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			calls := 0
			password, err := promptRecoveryPassword(&output, func() ([]byte, error) {
				calls++
				if tc.readError {
					return nil, errors.New("terminal disconnected")
				}
				if calls == 1 {
					return []byte(tc.first), nil
				}
				return []byte(tc.second), nil
			})
			if (err != nil) != tc.wantError {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tc.wantError && password != tc.first {
				t.Fatal("password changed by prompt")
			}
			if tc.first != "" && strings.Contains(output.String(), tc.first) {
				t.Fatal("password leaked to terminal output")
			}
		})
	}
}

func (write startupLogWriter) Write(data []byte) (int, error) { return write(data) }

func TestStartupLogsSetupTokenOnlyBeforeFirstAccount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.db")
	t.Setenv(settings.CampfireS3Bucket.EnvironmentName, "")

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

		if err := Run(ctx, []string{"--" + settings.CampfireDatabase.FlagName, path, "--" + settings.CampfireListen.FlagName, "127.0.0.1:0", "--" + settings.CampfirePublicURL.FlagName, "", "--" + settings.CampfireAgentListen.FlagName, "", "--" + settings.CampfireOIDCIssuer.FlagName, "", "--" + settings.CampfireOIDCClientID.FlagName, "", "--" + settings.CampfireOIDCClientSecret.FlagName, "", "--" + settings.CampfireDisableLocalLogin.FlagName + "=false"}); err != nil {
			t.Fatal(err)
		}

		return output.String()
	}
	output := run()

	var token string

	for line := range strings.SplitSeq(strings.TrimSpace(output), "\n") {
		var event struct {
			Message string `json:"msg"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}

		if value, ok := strings.CutPrefix(event.Message, "Administrator setup token: "); ok {
			if token != "" {
				t.Fatal("setup token printed more than once")
			}

			token = value
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
	_, createErr := auth.Access().CreateUser(context.Background(), service.CreateUserInput{Username: "admin", Password: "a long administrator test password A1!", RoleID: database.AdminRole, Disabled: false, Name: "", Email: ""})

	closeErr := db.Close()
	if createErr != nil || closeErr != nil {
		t.Fatal(createErr, closeErr)
	}

	if output := run(); strings.Contains(output, "Administrator setup token:") || strings.Contains(output, token) {
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
	if err := store.Setup(ctx, database.User{ID: 1, Username: "admin", PasswordHash: "old", CredentialVersion: 1}, database.UserSession{Hash: "old-session", CSRF: "csrf", CredentialVersion: 1, ExpiresAt: time.Now().Add(time.Hour).Unix(), LastSeenAt: time.Now().Unix()}); err != nil {
		t.Fatal(err)
	}

	password := "a replacement passphrase with spaces A1!"

	file := filepath.Join(t.TempDir(), "password")
	if err := os.WriteFile(file, []byte(password+"\r\n"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := Run(ctx, []string{"reset-password", "--" + settings.CampfireDatabase.FlagName, path, "--" + settings.RecoveryPasswordFile.FlagName, file}); err != nil {
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

	if err := Run(ctx, []string{"reset-password", "--" + settings.CampfireDatabase.FlagName, path, "--" + settings.RecoveryPasswordFile.FlagName, file}); err == nil {
		t.Fatal("public password file accepted")
	}

	if err := Run(ctx, []string{"reset-password", "--" + settings.CampfireDatabase.FlagName, path}); err == nil {
		t.Fatal("missing password file accepted")
	}
}

func TestResetTwoFactorCommandPreservesPassword(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.db")
	db, err := database.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	auth := service.NewAuthService(repository.NewAuthRepository(db.ORM()))
	setup, err := auth.PrepareSetup(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	session, token, err := auth.Setup(t.Context(), setup, "admin", "test administrator password for recovery A1!")
	if err != nil {
		t.Fatal(err)
	}
	factor := database.UserMFA{UserID: session.UserID, SecretRef: "old-reference", RecoveryHashes: []string{"hashed-code"}}
	if err := db.ORM().Omit("User").Create(&factor).Error; err != nil {
		t.Fatal(err)
	}
	if err := Run(t.Context(), []string{"reset-2fa", "--" + settings.CampfireDatabase.FlagName, path, "--username", "admin"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := auth.Session(t.Context(), token); err == nil {
		t.Fatal("browser session not revoked")
	}
	if _, _, err := auth.Login(t.Context(), "admin", "test administrator password for recovery A1!", ""); err != nil {
		t.Fatal("password login not restored", err)
	}
	if err := db.ORM().Take(&factor, "user_id = ?", session.UserID).Error; err != nil || factor.SecretRef != "" || len(factor.RecoveryHashes) != 0 {
		t.Fatal("factor not removed", err)
	}
	if err := Run(t.Context(), []string{"reset-2fa", "--" + settings.CampfireDatabase.FlagName, path}); err == nil {
		t.Fatal("reset without explicit user accepted")
	}
}
