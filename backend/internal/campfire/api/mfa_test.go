package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/service"
	"campfire/internal/campfire/service/integrations"
)

func enableAPIMFA(t *testing.T, handler http.Handler, cookie *http.Cookie, csrf string, expected ...int) (service.MFAEnrollment, []string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"name": "Personal phone"})
	response := authRequest(handler, "POST", "/api/v1/auth/mfa/enroll", string(body), cookie, csrf, "")
	var enrollment service.MFAEnrollment
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &enrollment) != nil || !strings.Contains(response.Body.String(), "data:image/png;base64,") {
		t.Fatal(response.Code, response.Body)
	}
	code, err := service.TOTPCode(enrollment.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	response = authRequest(handler, "POST", "/api/v1/auth/mfa/confirm", fmt.Sprintf(`{"code":%q}`, code), cookie, csrf, "")
	var result struct {
		Codes []string `json:"recovery_codes"`
	}
	want := 10
	if len(expected) > 0 {
		want = expected[0]
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &result) != nil || len(result.Codes) != want {
		t.Fatal(response.Code, response.Body)
	}
	return enrollment, result.Codes
}

func TestMFALoginChallengeDoesNotIssueSession(t *testing.T) {
	auth, handler, setup := authFixture(t, "")
	auth.WithMFASecrets(integrations.Secrets{Directory: t.TempDir()})
	state, cookie := readAuthResponse(t, authRequest(handler, "POST", "/api/v1/auth/setup", setupBody(t, setup), nil, "", ""))
	_, codes := enableAPIMFA(t, handler, cookie, state.CSRF)
	body := fmt.Sprintf(`{"username":"admin","password":%q}`, testAdminPassword)
	response := authRequest(handler, "POST", "/api/v1/auth/login", body, nil, "", "")
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), `"two_factor_required":true`) || len(response.Result().Cookies()) != 0 {
		t.Fatal("password alone created a session", response.Code, response.Body)
	}
	response = authRequest(handler, "POST", "/api/v1/auth/login", fmt.Sprintf(`{"username":"admin","password":%q,"code":%q}`, testAdminPassword, codes[0]), nil, "", "")
	readAuthResponse(t, response)
	response = authRequest(handler, "POST", "/api/v1/auth/login", fmt.Sprintf(`{"username":"admin","password":%q,"code":%q}`, testAdminPassword, codes[0]), nil, "", "")
	if response.Code != http.StatusUnauthorized || len(response.Result().Cookies()) != 0 {
		t.Fatal("recovery replay created a session", response.Code, response.Body)
	}
	response = authRequest(handler, "GET", "/api/v1/auth/mfa", "", cookie, "", "")
	if strings.Contains(response.Body.String(), "secret") || strings.Contains(response.Body.String(), codes[1]) {
		t.Fatal("status leaked secrets")
	}
}

func TestMFARecoveryFromLogin(t *testing.T) {
	auth, handler, setup := authFixture(t, "")
	auth.WithMFASecrets(integrations.Secrets{Directory: t.TempDir()})
	state, cookie := readAuthResponse(t, authRequest(handler, "POST", "/api/v1/auth/setup", setupBody(t, setup), nil, "", ""))
	_, codes := enableAPIMFA(t, handler, cookie, state.CSRF)
	enableAPIMFA(t, handler, cookie, state.CSRF, 0)
	for _, credentials := range [][2]string{{"wrong", codes[0]}, {testAdminPassword, "invalid"}} {
		body := fmt.Sprintf(`{"username":"admin","password":%q,"code":%q}`, credentials[0], credentials[1])
		response := authRequest(handler, "POST", "/api/v1/auth/login", body, nil, "", "")
		if response.Code != http.StatusUnauthorized || len(response.Result().Cookies()) != 0 {
			t.Fatal("invalid recovery succeeded", response.Code, response.Body)
		}
	}
	body := fmt.Sprintf(`{"username":"admin","password":%q,"code":%q}`, testAdminPassword, codes[0])
	_, recoveredCookie := readAuthResponse(t, authRequest(handler, "POST", "/api/v1/auth/login", body, nil, "", ""))
	response := authRequest(handler, "GET", "/api/v1/auth/mfa", "", recoveredCookie, "", "")
	var status service.MFAStatus
	if json.Unmarshal(response.Body.Bytes(), &status) != nil || !status.Enabled || len(status.Devices) != 2 || status.RecoveryRemaining != 9 {
		t.Fatal(response.Body)
	}
	response = authRequest(handler, "GET", "/api/v1/auth/mfa", "", cookie, "", "")
	if response.Code != http.StatusOK {
		t.Fatal("recovery unexpectedly revoked another browser", response.Code)
	}
	response = authRequest(handler, "POST", "/api/v1/auth/login", body, nil, "", "")
	if response.Code != http.StatusUnauthorized {
		t.Fatal("recovery code reused", response.Code)
	}
	response = authRequest(handler, "POST", "/api/v1/auth/login", fmt.Sprintf(`{"username":"admin","password":%q}`, testAdminPassword), nil, "", "")
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), `"two_factor_required":true`) {
		t.Fatal("recovery disabled MFA", response.Code, response.Body)
	}
}

func TestAdminMFAResetRestoresPasswordLoginAndPreservesTokens(t *testing.T) {
	auth, handler, setup := authFixture(t, "")
	auth.WithMFASecrets(integrations.Secrets{Directory: t.TempDir()})
	adminState, adminCookie := readAuthResponse(t, authRequest(handler, "POST", "/api/v1/auth/setup", setupBody(t, setup), nil, "", ""))
	user, err := auth.Access().CreateUser(t.Context(), service.CreateUserInput{Username: "protected", Password: testAdminPassword, RoleID: database.UserRole})
	if err != nil {
		t.Fatal(err)
	}
	userSession, token, err := auth.Login(t.Context(), user.Username, testAdminPassword, "")
	if err != nil {
		t.Fatal(err)
	}
	userCookie := &http.Cookie{Name: "campfire_session", Value: token}
	// API tokens remain independent of browser MFA enrollment and recovery.
	response := authRequest(handler, "POST", "/api/v1/tokens", `{"name":"automation","expires_in_days":30,"scopes":["*"]}`, userCookie, userSession.CSRF, "")
	var apiToken struct {
		Secret string `json:"secret"`
	}
	if response.Code != http.StatusCreated || json.Unmarshal(response.Body.Bytes(), &apiToken) != nil {
		t.Fatal(response.Code, response.Body)
	}
	enableAPIMFA(t, handler, userCookie, userSession.CSRF)
	endpoint := fmt.Sprintf("/api/v1/users/%d/reset-2fa", user.ID)
	response = authRequest(handler, "POST", endpoint, `{}`, userCookie, userSession.CSRF, "")
	if response.Code != http.StatusForbidden {
		t.Fatal("ordinary user can reset MFA", response.Code)
	}
	response = authRequest(handler, "POST", endpoint, `{}`, adminCookie, "", "")
	if response.Code != http.StatusForbidden {
		t.Fatal("reset without CSRF", response.Code)
	}
	response = authRequest(handler, "POST", endpoint, `{}`, adminCookie, adminState.CSRF, "")
	if response.Code != http.StatusNoContent {
		t.Fatal(response.Code, response.Body)
	}
	response = authRequest(handler, "GET", "/api/v1/auth/mfa", "", userCookie, "", "")
	if response.Code != http.StatusUnauthorized {
		t.Fatal("reset left old session active", response.Code)
	}
	if _, _, err := auth.Login(t.Context(), user.Username, testAdminPassword, ""); err != nil {
		t.Fatal("reset still requires MFA", err)
	}
	if _, err := auth.AuthenticateAPIToken(t.Context(), apiToken.Secret); err != nil {
		t.Fatal("reset revoked API token", err)
	}
}

func TestMFAManagementRejectsPreviewAndBearer(t *testing.T) {
	f := newAccessFixture(t)
	user, browser := f.user("mfa-owner", database.UserRole)
	_, preview := previewAs(t, f.admin, user.ID)
	_, secret := createTestAPIToken(t, f.admin, `["*"]`)
	for _, handler := range []http.Handler{preview, bearerAPI(f.panel, secret)} {
		expectAccess(t, handler, "GET", "/api/v1/auth/mfa", "", http.StatusForbidden)
		expectAccess(t, handler, "POST", "/api/v1/auth/mfa/enroll", `{}`, http.StatusForbidden)
		expectAccess(t, handler, "DELETE", "/api/v1/auth/mfa", `{}`, http.StatusForbidden)
		expectAccess(t, handler, "DELETE", "/api/v1/auth/mfa/devices/unknown", `{}`, http.StatusForbidden)
		expectAccess(t, handler, "POST", fmt.Sprintf("/api/v1/users/%d/reset-2fa", user.ID), `{}`, http.StatusForbidden)
	}
	expectAccess(t, browser, "POST", "/api/v1/users/1/reset-2fa", `{}`, http.StatusForbidden)
}

func TestMFADeviceEnrollmentAndRemoval(t *testing.T) {
	auth, handler, setup := authFixture(t, "")
	auth.WithMFASecrets(integrations.Secrets{Directory: t.TempDir()})
	state, cookie := readAuthResponse(t, authRequest(handler, "POST", "/api/v1/auth/setup", setupBody(t, setup), nil, "", ""))
	response := authRequest(handler, "POST", "/api/v1/auth/mfa/enroll", `{"name":"Phone"}`, cookie, "", "")
	if response.Code != http.StatusForbidden {
		t.Fatal("enrollment bypassed CSRF", response.Code)
	}
	_, codes := enableAPIMFA(t, handler, cookie, state.CSRF)
	_, more := enableAPIMFA(t, handler, cookie, state.CSRF, 0)
	if len(more) != 0 {
		t.Fatal("second device replaced recovery codes")
	}
	response = authRequest(handler, "GET", "/api/v1/auth/mfa", "", cookie, "", "")
	var status service.MFAStatus
	if json.Unmarshal(response.Body.Bytes(), &status) != nil || len(status.Devices) != 2 {
		t.Fatal(response.Body)
	}
	endpoint := "/api/v1/auth/mfa/devices/" + status.Devices[0].ID
	response = authRequest(handler, "DELETE", endpoint, `{"code":"invalid"}`, cookie, state.CSRF, "")
	if response.Code != http.StatusBadRequest {
		t.Fatal("invalid proof accepted", response.Code)
	}
	response = authRequest(handler, "DELETE", endpoint, fmt.Sprintf(`{"code":%q}`, codes[0]), cookie, state.CSRF, "")
	if response.Code != http.StatusNoContent {
		t.Fatal(response.Code, response.Body)
	}
	response = authRequest(handler, "GET", "/api/v1/auth/mfa", "", cookie, "", "")
	if json.Unmarshal(response.Body.Bytes(), &status) != nil || len(status.Devices) != 1 || !status.Enabled {
		t.Fatal(response.Body)
	}
}
