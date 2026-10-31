package service

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/utils"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// OIDCConfig configures a single optional OpenID Connect provider.
type OIDCConfig struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	PublicURL    string
	DisableLocal bool
}

type oidcAttempt struct {
	state, nonce, verifier string
	sessionHash            string
	expires                time.Time
	userID, version        int64
}

// OIDCService owns provider validation and short-lived, single-use browser flows.
type OIDCService struct {
	auth     *AuthService
	issuer   string
	oauth    oauth2.Config
	verifier *oidc.IDTokenVerifier
	provider *oidc.Provider
	userInfo bool
	client   *http.Client
	mu       sync.Mutex
	attempts map[string]oidcAttempt
}

// NewOIDCService discovers the configured provider and checks that login remains possible.
func NewOIDCService(ctx context.Context, auth *AuthService, config OIDCConfig) (*OIDCService, error) {
	if config.Issuer == "" {
		if config.ClientID != "" || config.ClientSecret != "" {
			return nil, errors.New("OIDC issuer and client ID are required when configuring OIDC")
		}

		if auth.store != nil {
			if err := auth.store.PruneLoginProviders(ctx, "", false); err != nil {
				return nil, err
			}
		}

		auth.loginProvider = ""
		auth.disableLocal = false

		return nil, nil
	}

	issuer, err := url.Parse(config.Issuer)
	if err != nil || !oidcURL(issuer) || issuer.RawQuery != "" || issuer.Fragment != "" || config.ClientID == "" {
		return nil, errors.New("OIDC requires a valid HTTPS issuer and client ID (HTTP is allowed on loopback)")
	}

	public, err := url.Parse(config.PublicURL)
	if err != nil || !oidcURL(public) || public.RawQuery != "" || public.Fragment != "" || public.Path != "" && public.Path != "/" {
		return nil, errors.New("OIDC requires an explicit public URL using HTTPS (HTTP is allowed on loopback)")
	}

	client := &http.Client{Timeout: 15 * time.Second}
	providerContext := oidc.ClientContext(ctx, client)

	provider, err := oidc.NewProvider(providerContext, config.Issuer)
	if err != nil {
		return nil, fmt.Errorf("discover OIDC provider: %w", err)
	}

	endpoint := provider.Endpoint()

	var metadata struct {
		KeysURL     string `json:"jwks_uri"`
		UserInfoURL string `json:"userinfo_endpoint"`
	}
	if err := provider.Claims(&metadata); err != nil {
		return nil, errors.New("invalid OIDC provider metadata")
	}

	for _, value := range []string{endpoint.AuthURL, endpoint.TokenURL, metadata.KeysURL} {
		parsed, err := url.Parse(value)
		if err != nil || !oidcURL(parsed) {
			return nil, errors.New("OIDC provider endpoints must use HTTPS (HTTP is allowed on loopback)")
		}
	}

	if metadata.UserInfoURL != "" {
		parsed, err := url.Parse(metadata.UserInfoURL)
		if err != nil || !oidcURL(parsed) {
			return nil, errors.New("OIDC UserInfo endpoint must use HTTPS (HTTP is allowed on loopback)")
		}
	}

	disableLocal := false

	if config.DisableLocal {
		ready, err := auth.store.HasOIDCAdmin(ctx, config.Issuer)
		if err != nil {
			return nil, err
		}

		disableLocal = ready
	}

	if err := auth.store.PruneLoginProviders(ctx, config.Issuer, disableLocal); err != nil {
		return nil, err
	}

	auth.loginProvider = config.Issuer
	auth.disableLocal = config.DisableLocal

	return &OIDCService{auth: auth, issuer: config.Issuer, client: client, provider: provider, userInfo: metadata.UserInfoURL != "", attempts: make(map[string]oidcAttempt), verifier: provider.Verifier(&oidc.Config{ClientID: config.ClientID}), oauth: oauth2.Config{
		ClientID: config.ClientID, ClientSecret: config.ClientSecret, Endpoint: endpoint,
		RedirectURL: strings.TrimRight(config.PublicURL, "/") + "/api/v1/auth/oidc/callback",
		Scopes:      []string{oidc.ScopeOpenID, "profile", "email"},
	}}, nil
}

func oidcURL(value *url.URL) bool {
	return value != nil && value.Host != "" && value.User == nil && (value.Scheme == "https" || value.Scheme == "http" && (value.Hostname() == "localhost" || value.Hostname() == "127.0.0.1" || value.Hostname() == "::1"))
}

// Start creates a flow bound to a private browser cookie, with PKCE and nonce.
func (s *OIDCService) Start(ctx context.Context, linkToken string) (string, string, error) {
	var session database.UserSession

	if linkToken != "" {
		var err error

		session, _, err = s.auth.Session(ctx, linkToken)
		if err != nil {
			return "", "", err
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for key, attempt := range s.attempts {
		if !attempt.expires.After(now) {
			delete(s.attempts, key)
		}
	}

	if len(s.attempts) >= 1000 {
		return "", "", ErrAuthRate
	}

	cookie := authToken()
	attempt := oidcAttempt{state: authToken(), nonce: authToken(), verifier: oauth2.GenerateVerifier(), expires: now.Add(5 * time.Minute), userID: session.UserID, version: session.CredentialVersion, sessionHash: tokenHash(linkToken)}
	s.attempts[tokenHash(cookie)] = attempt

	return s.oauth.AuthCodeURL(attempt.state, oidc.Nonce(attempt.nonce), oauth2.S256ChallengeOption(attempt.verifier)), cookie, nil
}

// Complete consumes a flow before exchanging its code and validating the signed ID token.
func (s *OIDCService) Complete(ctx context.Context, cookie, state, code string) (database.UserSession, string, error) {
	s.mu.Lock()
	attempt, ok := s.attempts[tokenHash(cookie)]
	delete(s.attempts, tokenHash(cookie))
	s.mu.Unlock()

	if !ok || !attempt.expires.After(time.Now()) || subtle.ConstantTimeCompare([]byte(attempt.state), []byte(state)) != 1 || code == "" {
		return database.UserSession{}, "", ErrAuthentication
	}

	if attempt.userID != 0 {
		if _, _, err := s.auth.store.Session(ctx, attempt.sessionHash, s.auth.now()); err != nil {
			return database.UserSession{}, "", ErrAuthentication
		}
	}

	ctx = oidc.ClientContext(ctx, s.client)

	token, err := s.oauth.Exchange(ctx, code, oauth2.VerifierOption(attempt.verifier))
	if err != nil {
		return database.UserSession{}, "", ErrAuthentication
	}

	raw, ok := token.Extra("id_token").(string)
	if !ok {
		return database.UserSession{}, "", ErrAuthentication
	}

	id, err := s.verifier.Verify(ctx, raw)
	if err != nil || id.Subject == "" || len(id.Subject) > 255 || subtle.ConstantTimeCompare([]byte(id.Nonce), []byte(attempt.nonce)) != 1 {
		return database.UserSession{}, "", ErrAuthentication
	}

	var claims struct {
		AuthorizedParty string `json:"azp"`
		Username        string `json:"preferred_username"`
		Name            string `json:"name"`
		Email           string `json:"email"`
		Picture         string `json:"picture"`
	}
	if err := id.Claims(&claims); err != nil || claims.AuthorizedParty != "" && claims.AuthorizedParty != s.oauth.ClientID || len(id.Audience) > 1 && claims.AuthorizedParty != s.oauth.ClientID {
		return database.UserSession{}, "", ErrAuthentication
	}

	if s.userInfo {
		info, err := s.provider.UserInfo(ctx, oauth2.StaticTokenSource(token))
		if err != nil || info.Subject != id.Subject {
			return database.UserSession{}, "", ErrAuthentication
		}

		var profile struct {
			Name    string `json:"name"`
			Email   string `json:"email"`
			Picture string `json:"picture"`
		}
		if err := info.Claims(&profile); err != nil {
			return database.UserSession{}, "", ErrAuthentication
		}

		if profile.Name != "" {
			claims.Name = profile.Name
		}

		if profile.Email != "" {
			claims.Email = profile.Email
		}

		if profile.Picture != "" {
			claims.Picture = profile.Picture
		}
	}

	// A suffix prevents a claim from selecting an existing local account.
	name := strings.Map(func(r rune) rune {
		if utils.IsValidIdentifierCharacter(r) {
			return r
		}

		return -1
	}, strings.ToLower(claims.Username))
	if len(name) > 40 {
		name = name[:40]
	}

	name = strings.Trim(name, "._-")
	if name == "" {
		name = "oidc"
	}

	displayName := strings.TrimSpace(claims.Name)
	if !validDisplayName(displayName) {
		displayName = ""
	}

	email := strings.TrimSpace(claims.Email)
	if !validEmail(email) {
		email = ""
	}

	picture := ""
	if parsed, err := url.Parse(claims.Picture); err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil && len(claims.Picture) <= 2048 {
		picture = claims.Picture
	}

	user, err := s.auth.store.OIDCAccount(ctx, s.issuer, id.Subject, name+"-"+tokenHash(authToken())[:12], attempt.userID, attempt.version, attempt.sessionHash, displayName, email, picture)
	if err != nil {
		return database.UserSession{}, "", err
	}

	session, rawSession := newUserSession(user.ID, user.CredentialVersion, s.auth.now())
	session.Provider = s.issuer

	if err := s.auth.store.CreateSession(ctx, session, "", s.auth.now()); err != nil {
		return database.UserSession{}, "", err
	}

	localLogin, err := s.auth.LocalLoginEnabled(ctx)
	if err != nil {
		return database.UserSession{}, "", err
	}

	if !localLogin {
		if err := s.auth.store.PruneLoginProviders(ctx, s.issuer, true); err != nil {
			return database.UserSession{}, "", err
		}
	}

	session, _, err = s.auth.Session(ctx, rawSession)

	return session, rawSession, err
}
