package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/service"
)

func listedSessions(t *testing.T, handler http.Handler) []service.LoginSession {
	t.Helper()
	response := request(handler, "GET", "/api/v1/auth/sessions", "")
	var sessions []service.LoginSession
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &sessions) != nil {
		t.Fatal(response.Code, response.Body)
	}
	for _, secret := range []string{`"hash"`, `"csrf"`, `"credential_version"`} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatal("session secret leaked", response.Body)
		}
	}
	return sessions
}

func TestSessionManagementOwnershipAndRevocation(t *testing.T) {
	f := newAccessFixture(t)
	_, browser := f.user("session-owner", database.UserRole)
	_, other := f.user("session-other", database.UserRole)
	sessions := listedSessions(t, browser)
	if len(sessions) != 1 || !sessions[0].Current || sessions[0].CreatedAt == 0 {
		t.Fatal(sessions)
	}
	id := sessions[0].ID
	expectAccess(t, other, "DELETE", "/api/v1/auth/sessions/"+id, "", http.StatusNoContent)
	expectAccess(t, other, "DELETE", "/api/v1/auth/sessions", "", http.StatusNoContent)
	if len(listedSessions(t, browser)) != 1 {
		t.Fatal("another account revoked the owner's session")
	}
	session, token, err := f.auth.Login(t.Context(), "session-owner", testAdminPassword, "")
	if err != nil {
		t.Fatal(err)
	}
	second := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.AddCookie(&http.Cookie{Name: "campfire_session", Value: token})
		r.Header.Set("X-CSRF-Token", session.CSRF)
		f.panel.ServeHTTP(w, r)
	})
	if len(listedSessions(t, second)) != 2 {
		t.Fatal("missing second session")
	}
	expectAccess(t, second, "DELETE", "/api/v1/auth/sessions", "", http.StatusNoContent)
	expectAccess(t, browser, "GET", "/api/v1/auth/sessions", "", http.StatusUnauthorized)
	sessions = listedSessions(t, second)
	if len(sessions) != 1 || !sessions[0].Current {
		t.Fatal(sessions)
	}
	response := request(second, "DELETE", "/api/v1/auth/sessions/"+sessions[0].ID, "")
	if response.Code != http.StatusNoContent || len(response.Result().Cookies()) != 1 || response.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal(response.Code, response.Header())
	}
	expectAccess(t, second, "GET", "/api/v1/auth/sessions", "", http.StatusUnauthorized)
	expectAccess(t, other, "GET", "/api/v1/auth/sessions", "", http.StatusOK)
}

func TestSessionManagementRejectsPreviewTokensAndMissingCSRF(t *testing.T) {
	f := newAccessFixture(t)
	user, browser := f.user("sessions-reader", database.UserRole)
	_, preview := previewAs(t, f.admin, user.ID)
	_, secret := createTestAPIToken(t, browser, `["*"]`)
	for _, handler := range []http.Handler{preview, bearerAPI(f.panel, secret)} {
		expectAccess(t, handler, "GET", "/api/v1/auth/sessions", "", http.StatusForbidden)
		expectAccess(t, handler, "DELETE", "/api/v1/auth/sessions", "", http.StatusForbidden)
		expectAccess(t, handler, "DELETE", "/api/v1/auth/sessions/missing", "", http.StatusForbidden)
	}
	session, token, err := f.auth.Login(t.Context(), "sessions-reader", testAdminPassword, "")
	if err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: "campfire_session", Value: token}
	for _, path := range []string{"/api/v1/auth/sessions", "/api/v1/auth/sessions/missing"} {
		response := authRequest(f.panel, "DELETE", path, "", cookie, "", "")
		if response.Code != http.StatusForbidden {
			t.Fatal(response.Code, response.Body)
		}
		response = authRequest(f.panel, "DELETE", path, "", cookie, session.CSRF, "https://evil.example")
		if response.Code != http.StatusForbidden {
			t.Fatal(response.Code, response.Body)
		}
	}
}

func TestLoginCapturesSessionBrowserMetadata(t *testing.T) {
	_, handler, token := authFixture(t, "")
	req := httptest.NewRequest("POST", "/api/v1/auth/setup", strings.NewReader(setupBody(t, token)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Example browser")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	_, cookie := readAuthResponse(t, response)
	response = authRequest(handler, "GET", "/api/v1/auth/sessions", "", cookie, "", "")
	var sessions []service.LoginSession
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &sessions) != nil || len(sessions) != 1 || sessions[0].UserAgent != "Example browser" {
		t.Fatal(response.Code, response.Body)
	}
}
