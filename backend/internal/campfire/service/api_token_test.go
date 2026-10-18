package service

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
)

func TestAPITokenLifecycleAndAccountRevocation(t *testing.T) {
	ctx := context.Background()

	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	auth := NewAuthService(repository.NewAuthRepository(db.ORM()))

	user, err := auth.Access().CreateUser(ctx, "token-user", "a private long test password", database.UserRole, false, "", "")
	if err != nil {
		t.Fatal(err)
	}

	_, sessionSecret, err := auth.Login(ctx, user.Username, "a private long test password", "")
	if err != nil {
		t.Fatal(err)
	}

	session, _, err := auth.Session(ctx, sessionSecret)
	if err != nil {
		t.Fatal(err)
	}

	principal := PrincipalFromSession(session)
	now := time.Now()
	auth.now = func() time.Time { return now }

	token, raw, err := auth.CreateAPIToken(ctx, principal, " automation ", 1, []string{"*"})
	if err != nil {
		t.Fatal(err)
	}

	var stored database.APIToken
	if err := db.ORM().Take(&stored, token.ID).Error; err != nil {
		t.Fatal(err)
	}

	if stored.Hash != tokenHash(raw) || stored.Hash == raw || token.Name != "automation" || len(raw) != 46 {
		t.Fatal("incorrect secret storage", stored.Name)
	}

	encoded, err := json.Marshal(token)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(encoded), raw) || strings.Contains(string(encoded), stored.Hash) || strings.Contains(string(encoded), "credential_version") {
		t.Fatal("credential material exposed")
	}

	bearer, err := auth.AuthenticateAPIToken(ctx, raw)
	if err != nil || bearer.UserID != user.ID || bearer.APITokenID != token.ID || bearer.Allows("sparks.manage") {
		t.Fatal("token granted excess permissions", err)
	}

	if _, _, err := auth.CreateAPIToken(ctx, bearer, "nested", 1, []string{"*"}); !errors.Is(err, repository.ErrAccessDenied) {
		t.Fatal("bearer created a token", err)
	}

	if _, err := auth.APITokens(ctx, bearer); !errors.Is(err, repository.ErrAccessDenied) {
		t.Fatal("bearer listed tokens", err)
	}

	if err := auth.RevokeAPIToken(ctx, bearer, token.ID); !errors.Is(err, repository.ErrAccessDenied) {
		t.Fatal("bearer managed tokens", err)
	}

	auth.disableLocal = true
	if _, err := auth.AuthenticateAPIToken(ctx, raw); err != nil {
		t.Fatal("local login flag disabled API access", err)
	}

	now = now.Add(24 * time.Hour)

	if _, err := auth.AuthenticateAPIToken(ctx, raw); !errors.Is(err, ErrAuthentication) {
		t.Fatal("expired token accepted", err)
	}

	tokens, err := auth.APITokens(ctx, principal)
	if err != nil || len(tokens) != 0 {
		t.Fatal("expired token listed", err)
	}

	_, raw, err = auth.CreateAPIToken(ctx, principal, "replacement", 1, []string{"server.view"})
	if err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Model(&user).Update("credential_version", user.CredentialVersion+1).Error; err != nil {
		t.Fatal(err)
	}

	if _, err := auth.AuthenticateAPIToken(ctx, raw); !errors.Is(err, ErrAuthentication) {
		t.Fatal("changed credentials did not revoke token", err)
	}

	if _, _, err := auth.CreateAPIToken(ctx, principal, "stale", 1, []string{"*"}); !errors.Is(err, ErrAuthentication) {
		t.Fatal("stale principal minted token", err)
	}

	if err := auth.Access().DeleteUser(ctx, user.ID); err != nil {
		t.Fatal(err)
	}

	var count int64
	if err := db.ORM().Model(&database.APIToken{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("user deletion did not remove credentials", count, err)
	}
}

func TestAPITokenScopeValidationAndLimit(t *testing.T) {
	ctx := context.Background()

	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	auth := NewAuthService(repository.NewAuthRepository(db.ORM()))

	user, err := auth.Access().CreateUser(ctx, "token-user", "a private long test password", database.UserRole, false, "", "")
	if err != nil {
		t.Fatal(err)
	}

	principal := PrincipalFromSession(database.UserSession{UserID: user.ID, User: user})
	for _, scopes := range [][]string{nil, {"*", "server.view"}, {"unknown"}, {"server.operate"}} {
		if _, _, err := auth.CreateAPIToken(ctx, principal, "invalid", 1, scopes); !errors.Is(err, repository.ErrAccessInput) {
			t.Fatal("invalid scope accepted", scopes, err)
		}
	}

	for _, days := range []int{0, 366} {
		if _, _, err := auth.CreateAPIToken(ctx, principal, "invalid", days, []string{"*"}); !errors.Is(err, repository.ErrAccessInput) {
			t.Fatal("invalid expiry accepted", err)
		}
	}

	var first database.APIToken

	var secret string

	for i := range 50 {
		token, raw, err := auth.CreateAPIToken(ctx, principal, "automation", 1, []string{"server.view"})
		if err != nil {
			t.Fatal(i, err)
		}

		if i == 0 {
			first, secret = token, raw
		}
	}

	if _, _, err := auth.CreateAPIToken(ctx, principal, "overflow", 1, []string{"*"}); !errors.Is(err, repository.ErrAccessConflict) {
		t.Fatal("active token limit not enforced", err)
	}

	if err := auth.RevokeAPIToken(ctx, Principal{UserID: user.ID + 1}, first.ID); !errors.Is(err, repository.ErrAccessNotFound) {
		t.Fatal("another user revoked token", err)
	}

	if err := auth.RevokeAPIToken(ctx, principal, first.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := auth.AuthenticateAPIToken(ctx, secret); !errors.Is(err, ErrAuthentication) {
		t.Fatal("revoked token accepted", err)
	}

	if _, _, err := auth.CreateAPIToken(ctx, principal, "replacement", 1, []string{"*"}); err != nil {
		t.Fatal(err)
	}
}
