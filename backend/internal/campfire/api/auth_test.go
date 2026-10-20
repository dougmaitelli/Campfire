package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"campfire/internal/campfire/api"
	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
)

type testAuthState struct {
	Authenticated bool   `json:"authenticated"`
	SetupRequired bool   `json:"setup_required"`
	Username      string `json:"username"`
	CSRF          string `json:"csrf_token"`
}

func authRequest(handler http.Handler, method, path, body string, cookie *http.Cookie, csrf, origin string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	if cookie != nil {
		req.AddCookie(cookie)
	}

	req.Header.Set("X-CSRF-Token", csrf)
	req.Header.Set("Origin", origin)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)

	return response
}

func authFixture(t *testing.T, origin string) (*service.AuthService, http.Handler, string) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "panel.db")

	db, err := database.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	auth := service.NewAuthService(repository.NewAuthRepository(db.ORM()))

	token, err := auth.PrepareSetup(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	embers := service.NewEmberService(repository.NewEmberRepository(db.ORM()))
	servers := service.NewServerService(repository.NewServerRepository(db.ORM()), embers)
	sparks := service.NewSparkService(repository.NewSparkRepository(db.ORM()), nil)
	handler := api.New(sparks, embers, servers).WithAuthentication(auth, origin).Handler()

	return auth, handler, token
}

func setupBody(t *testing.T, token string) string {
	t.Helper()

	body, err := json.Marshal(map[string]string{"username": "Admin", "password": testAdminPassword, "setup_token": token})
	if err != nil {
		t.Fatal(err)
	}

	return string(body)
}

func readAuthResponse(t *testing.T, response *httptest.ResponseRecorder) (testAuthState, *http.Cookie) {
	t.Helper()

	if response.Code != http.StatusOK {
		t.Fatalf("authentication failed: %d %s", response.Code, response.Body)
	}

	var state testAuthState
	if err := json.Unmarshal(response.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}

	cookies := response.Result().Cookies()
	if len(cookies) != 1 || !state.Authenticated || state.CSRF == "" || state.Username != "admin" {
		t.Fatal("missing authenticated state or session cookie")
	}

	return state, cookies[0]
}

func TestAuthenticationBoundarySetupAndCSRF(t *testing.T) {
	_, handler, token := authFixture(t, "https://panel.example.com")
	for _, endpoint := range []struct{ method, path string }{
		{"GET", "/api/v1/sparks"}, {"POST", "/api/v1/sparks"},
		{"GET", "/api/v1/sparks/update-status"}, {"POST", "/api/v1/sparks/id/update"},
		{"POST", "/api/v1/sparks/id/prune"},
		{"GET", "/api/v1/embers"}, {"POST", "/api/v1/embers/preview-url"},
		{"GET", "/api/v1/servers"}, {"POST", "/api/v1/servers"},
		{"GET", "/api/v1/provisioning/access"},
		{"GET", "/api/v1/servers/id/console"}, {"POST", "/api/v1/servers/id/console"},
		{"GET", "/api/v1/servers/id/files"}, {"POST", "/api/v1/servers/id/files"},
		{"GET", "/api/v1/servers/id/backups"}, {"GET", "/api/v1/servers/id/backups/test.tar.gz"},
		{"POST", "/api/v1/servers/id/actions"}, {"DELETE", "/api/v1/servers/id"},
		{"POST", "/api/v1/sparks/id/enrollment"}, {"POST", "/api/v1/auth/logout"},
		{"GET", "/api/v1/auth/unknown"},
	} {
		response := authRequest(handler, endpoint.method, endpoint.path, "", nil, "", "")
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("unprotected %s %s: %d", endpoint.method, endpoint.path, response.Code)
		}
	}

	response := request(handler, "GET", "/api/v1/auth/session", "")

	var initial testAuthState
	if err := json.Unmarshal(response.Body.Bytes(), &initial); err != nil || !initial.SetupRequired || initial.Authenticated || initial.CSRF != "" || strings.Contains(response.Body.String(), token) {
		t.Fatal("invalid anonymous setup state", err)
	}

	if response := request(handler, "POST", "/api/v1/auth/setup", setupBody(t, "invalid")); response.Code != http.StatusUnauthorized {
		t.Fatal("setup did not require host access", response.Code)
	}

	for _, origin := range []string{"https://evil.example.com", "null", "http://panel.example.com"} {
		response := authRequest(handler, "POST", "/api/v1/auth/setup", setupBody(t, token), nil, "", origin)
		if response.Code != http.StatusForbidden {
			t.Fatal("cross-origin setup accepted", origin, response.Code)
		}
	}

	state, cookie := readAuthResponse(t, authRequest(handler, "POST", "/api/v1/auth/setup", setupBody(t, token), nil, "", "https://panel.example.com"))
	if !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/api" || cookie.MaxAge != 43200 {
		t.Fatalf("unsafe cookie attributes: %+v", cookie)
	}

	if response := request(handler, "POST", "/api/v1/auth/setup", setupBody(t, token)); response.Code != http.StatusUnauthorized && response.Code != http.StatusConflict {
		t.Fatal("setup token was reusable", response.Code)
	}

	if response := authRequest(handler, "GET", "/api/v1/sparks", "", cookie, "", ""); response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("session did not grant read access", response.Code)
	}

	for _, csrf := range []string{"", "wrong"} {
		response := authRequest(handler, "POST", "/api/v1/sparks", `{"name":"node"}`, cookie, csrf, "https://panel.example.com")
		if response.Code != http.StatusForbidden {
			t.Fatal("missing/invalid CSRF accepted", response.Code)
		}
	}

	response = authRequest(handler, "POST", "/api/v1/sparks", `{"name":"node"}`, cookie, state.CSRF, "https://evil.example.com")
	if response.Code != http.StatusForbidden {
		t.Fatal("valid CSRF bypassed origin guard", response.Code)
	}

	response = authRequest(handler, "POST", "/api/v1/sparks", `{"name":"node"}`, cookie, state.CSRF, "https://panel.example.com")
	if response.Code != http.StatusCreated {
		t.Fatal("authenticated mutation failed", response.Code, response.Body)
	}

	response = authRequest(handler, "POST", "/api/v1/auth/logout", "", cookie, "", "")
	if response.Code != http.StatusForbidden {
		t.Fatal("logout did not require CSRF", response.Code)
	}

	response = authRequest(handler, "POST", "/api/v1/auth/logout", "", cookie, state.CSRF, "https://panel.example.com")
	if response.Code != http.StatusNoContent || len(response.Result().Cookies()) != 1 || response.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal("logout failed to clear cookie", response.Code)
	}

	if response := authRequest(handler, "GET", "/api/v1/sparks", "", cookie, "", ""); response.Code != http.StatusUnauthorized {
		t.Fatal("logged-out cookie remained valid", response.Code)
	}
}

func TestAuthenticationLoginRotationResetAndRateLimit(t *testing.T) {
	auth, handler, token := authFixture(t, "")
	state, cookie := readAuthResponse(t, request(handler, "POST", "/api/v1/auth/setup", setupBody(t, token)))

	for _, body := range []string{`{"username":"admin","password":"incorrect"}`, `{"username":"missing","password":"incorrect"}`} {
		if response := request(handler, "POST", "/api/v1/auth/login", body); response.Code != http.StatusUnauthorized {
			t.Fatal("invalid credentials accepted", response.Code)
		}
	}

	body := setupBody(t, "")

	rotated, newCookie := readAuthResponse(t, authRequest(handler, "POST", "/api/v1/auth/login", body, cookie, "", ""))
	if cookie.Value == newCookie.Value || state.CSRF == rotated.CSRF || newCookie.Secure {
		t.Fatal("session rotation or loopback cookie attributes failed")
	}

	if response := authRequest(handler, "GET", "/api/v1/sparks", "", cookie, "", ""); response.Code != http.StatusUnauthorized {
		t.Fatal("session was not rotated", response.Code)
	}

	if err := auth.ResetPassword(context.Background(), "a different long administrator password"); err != nil {
		t.Fatal(err)
	}

	if response := authRequest(handler, "GET", "/api/v1/sparks", "", newCookie, "", ""); response.Code != http.StatusUnauthorized {
		t.Fatal("password reset did not revoke sessions", response.Code)
	}

	if response := request(handler, "POST", "/api/v1/auth/login", body); response.Code != http.StatusUnauthorized {
		t.Fatal("old password remained valid", response.Code)
	}

	readAuthResponse(t, request(handler, "POST", "/api/v1/auth/login", `{"username":"admin","password":"a different long administrator password"}`))

	var throttled bool

	for range 12 {
		response := request(handler, "POST", "/api/v1/auth/login", `{"username":"admin","password":"wrong"}`)
		if response.Code == http.StatusTooManyRequests && response.Header().Get("Retry-After") == "60" {
			throttled = true
			break
		}
	}

	if !throttled {
		t.Fatal("login attempts were not rate limited")
	}

	_, _, err := auth.Session(context.Background(), "forged")
	if !errors.Is(err, service.ErrAuthentication) {
		t.Fatal("forged session accepted", err)
	}
}

func TestAuthenticationFailsClosedAndRejectsCrossSiteLogin(t *testing.T) {
	if response := request(api.New(nil, nil, nil).Handler(), "GET", "/api/v1/servers", ""); response.Code != http.StatusServiceUnavailable {
		t.Fatal("API without authentication failed open")
	}

	_, handler, token := authFixture(t, "")
	req := httptest.NewRequestWithContext(context.Background(), "POST", "/api/v1/auth/setup", strings.NewReader(setupBody(t, token)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.Header.Set("X-Forwarded-For", "127.0.0.1")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)

	if response.Code != http.StatusForbidden {
		t.Fatal("cross-site setup with forged forwarding accepted", response.Code)
	}

	req.Header.Del("Sec-Fetch-Site")
	req.Header.Set("Content-Type", "text/plain")

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, req)

	if response.Code != http.StatusUnsupportedMediaType {
		t.Fatal("simple cross-site form accepted", response.Code)
	}
}

func TestDevelopmentProxyOriginAndForwardedHeaderIsolation(t *testing.T) {
	_, handler, token := authFixture(t, "")
	req := httptest.NewRequestWithContext(context.Background(), "POST", "/api/v1/auth/setup", strings.NewReader(setupBody(t, token)))
	req.Host = "127.0.0.1:8081"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://127.0.0.1:8081")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)

	_, cookie := readAuthResponse(t, response)
	if cookie.Secure {
		t.Fatal("loopback development issued an unusable Secure cookie")
	}

	req = httptest.NewRequestWithContext(context.Background(), "POST", "/api/v1/auth/login", strings.NewReader(setupBody(t, "")))
	req.Host = "127.0.0.1:8081"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://panel.example.com")
	req.Header.Set("X-Forwarded-Host", "panel.example.com")
	req.Header.Set("X-Forwarded-Proto", "https")

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, req)

	if response.Code != http.StatusForbidden {
		t.Fatal("unconfigured forwarded origin was trusted", response.Code)
	}
}
