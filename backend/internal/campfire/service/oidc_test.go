package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"

	"github.com/go-jose/go-jose/v4"
)

type testOIDCProvider struct {
	server      *httptest.Server
	mu          sync.Mutex
	claims      map[string]any
	challenge   string
	signer      jose.Signer
	beforeToken func()
}

func newTestOIDCProvider(t *testing.T) *testOIDCProvider {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithHeader("kid", "test"))
	if err != nil {
		t.Fatal(err)
	}

	p := &testOIDCProvider{signer: signer}
	p.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": p.server.URL, "authorization_endpoint": p.server.URL + "/authorize", "token_endpoint": p.server.URL + "/token", "jwks_uri": p.server.URL + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"}})
		case "/keys":
			_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "test", Algorithm: "RS256", Use: "sig"}}})
		case "/token":
			p.mu.Lock()
			defer p.mu.Unlock()

			if err := r.ParseForm(); err != nil {
				http.Error(w, "bad form", http.StatusBadRequest)
				return
			}

			digest := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if base64.RawURLEncoding.EncodeToString(digest[:]) != p.challenge || r.Form.Get("code") != "valid" || r.Form.Get("redirect_uri") != "http://localhost:8080/api/v1/auth/oidc/callback" {
				http.Error(w, "invalid exchange", http.StatusBadRequest)
				return
			}

			if p.beforeToken != nil {
				p.beforeToken()
			}

			payload, err := json.Marshal(p.claims)
			if err != nil {
				t.Error(err)
				return
			}

			object, err := p.signer.Sign(payload)
			if err != nil {
				t.Error(err)
				return
			}

			token, err := object.CompactSerialize()
			if err != nil {
				t.Error(err)
				return
			}

			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "unused", "token_type": "Bearer", "id_token": token})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(p.server.Close)

	return p
}

func oidcFixture(t *testing.T) (*AuthService, *database.Database, *testOIDCProvider, OIDCConfig) {
	t.Helper()

	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	auth := NewAuthService(repository.NewAuthRepository(db.ORM()))
	if err := db.ORM().Omit("Role").Create(&database.User{Username: "admin", CredentialVersion: 1, RoleID: database.AdminRole}).Error; err != nil {
		t.Fatal(err)
	}

	p := newTestOIDCProvider(t)

	return auth, db, p, OIDCConfig{Issuer: p.server.URL, ClientID: "campfire", ClientSecret: "secret", PublicURL: "http://localhost:8080"}
}

func (p *testOIDCProvider) flow(t *testing.T, service *OIDCService, userID int64, subject string, changes map[string]any) (string, string) {
	t.Helper()

	var linkToken string

	if userID != 0 {
		session, raw := newUserSession(userID, 1, time.Now())

		session.Provider = service.issuer
		if err := service.auth.store.CreateSession(context.Background(), session, "", time.Now()); err != nil {
			t.Fatal(err)
		}

		linkToken = raw
	}

	target, cookie, err := service.Start(context.Background(), linkToken)
	if err != nil {
		t.Fatal(err)
	}

	parsed, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}

	query := parsed.Query()
	if query.Get("code_challenge_method") != "S256" || query.Get("nonce") == "" || query.Get("state") == "" || query.Get("response_type") != "code" {
		t.Fatal("missing code flow protections", query)
	}

	p.mu.Lock()
	p.challenge = query.Get("code_challenge")

	p.claims = map[string]any{"iss": p.server.URL, "sub": subject, "aud": "campfire", "exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(), "nonce": query.Get("nonce"), "preferred_username": "Admin", "email": "admin@example.com"}
	for key, value := range changes {
		p.claims[key] = value
	}
	p.mu.Unlock()

	return cookie, query.Get("state")
}

func TestOIDCProvisioningLinkingAndLocalPolicy(t *testing.T) {
	ctx := context.Background()
	auth, db, p, config := oidcFixture(t)

	provider, err := NewOIDCService(ctx, auth, config)
	if err != nil {
		t.Fatal(err)
	}

	cookie, state := p.flow(t, provider, 0, "regular-subject", map[string]any{"name": "OIDC User", "picture": "https://example.com/avatar.png"})

	session, raw, err := provider.Complete(ctx, cookie, state, "valid")
	if err != nil {
		t.Fatal(err)
	}

	if session.User.RoleID != database.UserRole || session.UserID == 1 || session.User.PasswordHash != "" || !strings.HasPrefix(session.User.Username, "admin-") || session.Provider != config.Issuer {
		t.Fatal("OIDC claims selected or elevated an existing account", session.UserID, session.User.RoleID)
	}

	if session.User.Name != "OIDC User" || session.User.Picture != "https://example.com/avatar.png" {
		t.Fatal("OIDC profile was not saved", session.User)
	}

	if _, _, err := provider.Complete(ctx, cookie, state, "valid"); !errors.Is(err, ErrAuthentication) {
		t.Fatal("callback replay accepted", err)
	}

	cookie, state = p.flow(t, provider, 0, "regular-subject", map[string]any{"preferred_username": "changed"})

	repeated, _, err := provider.Complete(ctx, cookie, state, "valid")
	if err != nil || repeated.UserID != session.UserID {
		t.Fatal("subject did not retain account", err)
	}

	if repeated.User.Name != session.User.Name || repeated.User.Picture != session.User.Picture {
		t.Fatal("missing claims cleared the saved profile", repeated.User)
	}

	config.DisableLocal = true

	fallbackAuth := NewAuthService(auth.store)
	if _, err := NewOIDCService(ctx, fallbackAuth, config); err != nil {
		t.Fatal("unlinked admin should retain local login", err)
	}

	if enabled, err := fallbackAuth.LocalLoginEnabled(ctx); err != nil || !enabled {
		t.Fatal("local login unavailable without a linked admin", err)
	}

	cookie, state = p.flow(t, provider, 1, "regular-subject", nil)
	if _, _, err := provider.Complete(ctx, cookie, state, "valid"); !errors.Is(err, repository.ErrAccessConflict) {
		t.Fatal("another account's identity was reassigned", err)
	}

	cookie, state = p.flow(t, provider, 1, "admin-subject", nil)

	linked, adminRaw, err := provider.Complete(ctx, cookie, state, "valid")
	if err != nil || linked.UserID != 1 {
		t.Fatal("admin linking failed", err)
	}

	if enabled, err := fallbackAuth.LocalLoginEnabled(ctx); err != nil || enabled {
		t.Fatal("local login did not turn off after admin linking", err)
	}

	cookie, state = p.flow(t, provider, 1, "cancelled-link", nil)
	provider.mu.Lock()
	linkHash := provider.attempts[tokenHash(cookie)].sessionHash
	provider.mu.Unlock()

	if err := auth.store.Revoke(ctx, linkHash); err != nil {
		t.Fatal(err)
	}

	if _, _, err := provider.Complete(ctx, cookie, state, "valid"); !errors.Is(err, ErrAuthentication) {
		t.Fatal("link completed after its authorizing session was revoked", err)
	}

	cookie, state = p.flow(t, provider, 1, "cancelled-during-exchange", nil)
	provider.mu.Lock()
	linkHash = provider.attempts[tokenHash(cookie)].sessionHash
	provider.mu.Unlock()
	p.mu.Lock()
	p.beforeToken = func() {
		if err := auth.store.Revoke(ctx, linkHash); err != nil {
			t.Error(err)
		}
	}
	p.mu.Unlock()

	if _, _, err := provider.Complete(ctx, cookie, state, "valid"); !errors.Is(err, ErrAuthentication) {
		t.Fatal("link completed after logout during token exchange", err)
	}

	p.mu.Lock()
	p.beforeToken = nil
	p.mu.Unlock()

	localSession, localRaw := newUserSession(1, 1, time.Now())
	if err := auth.store.CreateSession(ctx, localSession, "", time.Now()); err != nil {
		t.Fatal(err)
	}

	if _, err := NewOIDCService(ctx, auth, config); err != nil {
		t.Fatal(err)
	}

	if _, _, err := auth.Login(ctx, "admin", "any password", ""); !errors.Is(err, ErrAuthentication) {
		t.Fatal("disabled password login accepted", err)
	}

	if _, _, err := auth.Session(ctx, localRaw); !errors.Is(err, ErrAuthentication) {
		t.Fatal("password session survived OIDC-only policy", err)
	}

	if _, _, err := auth.Session(ctx, adminRaw); err != nil {
		t.Fatal("OIDC admin lost access", err)
	}

	localOnly := NewAuthService(auth.store)
	if _, _, err := localOnly.Session(ctx, adminRaw); !errors.Is(err, ErrAuthentication) {
		t.Fatal("OIDC session accepted without its provider", err)
	}

	if _, _, err := localOnly.Session(ctx, localRaw); !errors.Is(err, ErrAuthentication) {
		t.Fatal("disallowed local session revived when passwords were reenabled", err)
	}

	if err := db.ORM().Omit("Role").Create(&database.User{Username: "local-admin", CredentialVersion: 1, RoleID: database.AdminRole}).Error; err != nil {
		t.Fatal(err)
	}

	if _, err := auth.Access().UpdateUser(ctx, 1, database.UserRole, false, "", nil, nil); !errors.Is(err, repository.ErrLastAdmin) {
		t.Fatal("last OIDC admin could be demoted", err)
	}

	if err := auth.Access().DeleteUser(ctx, 1); !errors.Is(err, repository.ErrLastAdmin) {
		t.Fatal("last OIDC admin could be deleted", err)
	}

	if _, err := auth.Access().UpdateUser(ctx, session.UserID, database.UserRole, true, "", nil, nil); err != nil {
		t.Fatal(err)
	}

	if _, _, err := auth.Session(ctx, raw); !errors.Is(err, ErrAuthentication) {
		t.Fatal("disabled user's OIDC session remained valid", err)
	}

	cookie, state = p.flow(t, provider, 0, "regular-subject", nil)
	if _, _, err := provider.Complete(ctx, cookie, state, "valid"); !errors.Is(err, ErrAuthentication) {
		t.Fatal("disabled OIDC account signed in", err)
	}
}

func TestOIDCRejectsInvalidFlowsAndTokens(t *testing.T) {
	auth, _, p, config := oidcFixture(t)

	provider, err := NewOIDCService(context.Background(), auth, config)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name                                  string
		claims                                map[string]any
		badState, badCookie, expired, badPKCE bool
	}{
		{name: "state", badState: true},
		{name: "browser binding", badCookie: true},
		{name: "expired flow", expired: true},
		{name: "PKCE", badPKCE: true},
		{name: "nonce", claims: map[string]any{"nonce": "wrong"}},
		{name: "issuer", claims: map[string]any{"iss": "https://other.example.com"}},
		{name: "audience", claims: map[string]any{"aud": "another-client"}},
		{name: "expired token", claims: map[string]any{"exp": time.Now().Add(-time.Hour).Unix()}},
		{name: "empty subject", claims: map[string]any{"sub": ""}},
		{name: "authorized party", claims: map[string]any{"azp": "another-client"}},
		{name: "multiple audiences", claims: map[string]any{"aud": []string{"campfire", "other"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cookie, state := p.flow(t, provider, 0, "subject", tc.claims)
			if tc.badPKCE {
				p.mu.Lock()
				p.challenge = "wrong"
				p.mu.Unlock()
			}

			if tc.expired {
				provider.mu.Lock()
				attempt := provider.attempts[tokenHash(cookie)]
				attempt.expires = time.Now().Add(-time.Second)
				provider.attempts[tokenHash(cookie)] = attempt
				provider.mu.Unlock()
			}

			if tc.badState {
				state = "wrong"
			}

			if tc.badCookie {
				cookie = "wrong"
			}

			if _, _, err := provider.Complete(context.Background(), cookie, state, "valid"); !errors.Is(err, ErrAuthentication) {
				t.Fatal("invalid flow authenticated", err)
			}
		})
	}
}

func TestOIDCConfigurationValidation(t *testing.T) {
	auth := NewAuthService(nil)
	if provider, err := NewOIDCService(context.Background(), auth, OIDCConfig{DisableLocal: true}); err != nil || provider != nil {
		t.Fatal("OIDC should be optional", err)
	}

	if enabled, err := auth.LocalLoginEnabled(context.Background()); err != nil || !enabled {
		t.Fatal("local login must remain available without OIDC", err)
	}

	for _, config := range []OIDCConfig{
		{ClientID: "orphan"},
		{Issuer: "http://identity.example.com", ClientID: "campfire", PublicURL: "https://panel.example.com"},
		{Issuer: "https://identity.example.com", ClientID: "campfire"},
		{Issuer: "https://identity.example.com", PublicURL: "https://panel.example.com"},
		{Issuer: "https://identity.example.com", ClientID: "campfire", PublicURL: "http://panel.example.com"},
	} {
		if _, err := NewOIDCService(context.Background(), auth, config); err == nil {
			t.Fatal("invalid OIDC configuration accepted")
		}
	}
}
