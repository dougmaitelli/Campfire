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

func TestOIDCRoutesAndDisabledLocalLogin(t *testing.T) {
	ctx := context.Background()

	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	auth := service.NewAuthService(repository.NewAuthRepository(db.ORM()))

	user, err := auth.Access().CreateUser(ctx, "admin", testAdminPassword, database.AdminRole, false, "", "")
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

	config := service.OIDCConfig{Issuer: issuer, ClientID: "campfire", PublicURL: "https://panel.example.com"}

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
}
