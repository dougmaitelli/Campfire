package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"campfire/internal/campfire/api"
	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
)

func TestOIDCDisabledLocalLoginAllowsInitialSetup(t *testing.T) {
	ctx := context.Background()
	auth, _, token := authFixture(t, "")

	var issuer string

	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"issuer": issuer, "authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token", "jwks_uri": issuer + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"}})
	}))
	t.Cleanup(providerServer.Close)
	issuer = providerServer.URL

	provider, err := service.NewOIDCService(ctx, auth, service.OIDCConfig{Issuer: issuer, ClientID: "campfire", PublicURL: "https://panel.example.com", DisableLocal: true})
	if err != nil {
		t.Fatal(err)
	}

	handler := api.New(nil, nil, nil).WithAuthentication(auth, "").WithOIDC(provider).Handler()

	response := request(handler, "GET", "/api/v1/auth/session", "")
	if !strings.Contains(response.Body.String(), `"setup_required":true`) || !strings.Contains(response.Body.String(), `"local_login":true`) {
		t.Fatal("initial setup unavailable", response.Code, response.Body)
	}

	response = request(handler, "POST", "/api/v1/auth/setup", setupBody(t, token))
	if response.Code != http.StatusOK {
		t.Fatal("initial setup rejected", response.Code, response.Body)
	}

	response = request(handler, "POST", "/api/v1/auth/login", `{"username":"admin","password":"`+testAdminPassword+`"}`)
	if response.Code != http.StatusOK {
		t.Fatal("unlinked admin cannot log in", response.Code, response.Body)
	}
}

func TestOIDCRoutesAndDisabledLocalLogin(t *testing.T) {
	ctx := context.Background()

	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	auth := service.NewAuthService(repository.NewAuthRepository(db.ORM()))

	user, err := auth.Access().CreateUser(ctx, service.CreateUserInput{Username: "admin", Password: testAdminPassword, RoleID: database.AdminRole, Disabled: false, Name: "", Email: ""})
	if err != nil {
		t.Fatal(err)
	}

	local, raw, err := auth.Login(ctx, "admin", testAdminPassword, "")
	if err != nil {
		t.Fatal(err)
	}

	var issuer string

	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"issuer": issuer, "authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token", "jwks_uri": issuer + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"}})
	}))
	t.Cleanup(providerServer.Close)
	issuer = providerServer.URL

	config := service.OIDCConfig{Issuer: issuer, ClientID: "campfire", PublicURL: "https://panel.example.com", DisableLocal: false}

	provider, err := service.NewOIDCService(ctx, auth, config)
	if err != nil {
		t.Fatal(err)
	}

	panel := api.New(nil, nil, nil).WithAuthentication(auth, config.PublicURL).WithOIDC(provider)
	handler := panel.Handler()

	response := request(handler, "GET", "/api/v1/auth/session", "")
	if !strings.Contains(response.Body.String(), `"oidc_enabled":true`) || !strings.Contains(response.Body.String(), `"local_login":true`) {
		t.Fatal("login choices missing", response.Body)
	}

	response = request(handler, "GET", "/api/v1/auth/oidc/start", "")

	target, err := url.Parse(response.Header().Get("Location"))
	if err != nil || response.Code != http.StatusFound || target.Query().Get("client_id") != "campfire" || target.Query().Get("redirect_uri") != config.PublicURL+"/api/v1/auth/oidc/callback" {
		t.Fatal("invalid OIDC redirect", response.Code, response.Header())
	}

	cookies := response.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteLaxMode || cookies[0].MaxAge != 300 {
		t.Fatal("invalid OIDC flow cookie", cookies)
	}

	response = authRequest(handler, "GET", "/api/v1/auth/oidc/callback?state=wrong&code=invalid", "", cookies[0], "", "")
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/?oidc_error=1" {
		t.Fatal("invalid callback did not return to login", response.Code)
	}

	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == "campfire_session" || cookie.MaxAge != -1 {
			t.Fatal("invalid callback created a session or retained flow cookie")
		}
	}

	if response := request(handler, "POST", "/api/v1/auth/oidc/link", ""); response.Code != http.StatusUnauthorized {
		t.Fatal("anonymous account linking accepted", response.Code)
	}

	localCookie := &http.Cookie{Name: "campfire_session", Value: raw}
	response = authRequest(handler, "GET", "/api/v1/auth/session", "", localCookie, "", "")
	if !strings.Contains(response.Body.String(), `"oidc_linked":false`) {
		t.Fatal("unlinked account advertised as linked", response.Code, response.Body)
	}
	if response := authRequest(handler, "POST", "/api/v1/auth/oidc/link", "", localCookie, "", ""); response.Code != http.StatusForbidden {
		t.Fatal("account linking did not require CSRF", response.Code)
	}

	response = authRequest(handler, "POST", "/api/v1/auth/oidc/link", "", localCookie, local.CSRF, "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"url":`) {
		t.Fatal("authenticated linking failed", response.Code, response.Body)
	}

	if err := db.ORM().Omit("User").Create(&database.OIDCIdentity{Issuer: issuer, Subject: "admin", UserID: user.ID}).Error; err != nil {
		t.Fatal(err)
	}

	response = authRequest(handler, "GET", "/api/v1/auth/session", "", localCookie, "", "")
	if !strings.Contains(response.Body.String(), `"oidc_linked":true`) {
		t.Fatal("linked account advertised as unlinked", response.Code, response.Body)
	}

	// An OIDC browser session must be revoked by logout, including in OIDC-only mode.
	oidcSession, oidcRaw, err := auth.Login(ctx, "admin", testAdminPassword, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.ORM().Model(&database.UserSession{}).Where("hash = ?", oidcSession.Hash).Update("provider", issuer).Error; err != nil {
		t.Fatal(err)
	}
	oidcSessionCookie := &http.Cookie{Name: "campfire_session", Value: oidcRaw}

	config.DisableLocal = true
	if _, err := service.NewOIDCService(ctx, auth, config); err != nil {
		t.Fatal(err)
	}

	handler = panel.WithAuthentication(auth, config.PublicURL).Handler()

	response = request(handler, "GET", "/api/v1/auth/session", "")
	if !strings.Contains(response.Body.String(), `"local_login":false`) {
		t.Fatal("local login was advertised while disabled", response.Body)
	}

	for _, endpoint := range []string{"login", "setup"} {
		response := request(handler, "POST", "/api/v1/auth/"+endpoint, `{"username":"admin","password":"`+testAdminPassword+`"}`)
		if response.Code != http.StatusForbidden {
			t.Fatal("password endpoint usable while disabled", endpoint, response.Code)
		}
	}

	if response := authRequest(handler, "POST", "/api/v1/auth/oidc/link", "", localCookie, local.CSRF, ""); response.Code != http.StatusUnauthorized {
		t.Fatal("local session survived disabled login", response.Code)
	}

	response = authRequest(handler, "GET", "/api/v1/auth/session", "", oidcSessionCookie, "", "")
	if !strings.Contains(response.Body.String(), `"authenticated":true`) || !strings.Contains(response.Body.String(), `"oidc_linked":true`) {
		t.Fatal("OIDC session unavailable", response.Code, response.Body)
	}
	response = authRequest(handler, "POST", "/api/v1/auth/logout", "", oidcSessionCookie, oidcSession.CSRF, config.PublicURL)
	if response.Code != http.StatusNoContent {
		t.Fatal("OIDC logout failed", response.Code, response.Body)
	}
	cleared := false
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == "campfire_session" && cookie.Path == "/api" && cookie.MaxAge == -1 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("logout did not clear the session cookie")
	}
	// Even replaying the old cookie must not restore the session.
	response = authRequest(handler, "GET", "/api/v1/auth/session", "", oidcSessionCookie, "", "")
	if !strings.Contains(response.Body.String(), `"authenticated":false`) {
		t.Fatal("OIDC session survived logout", response.Code, response.Body)
	}
}
