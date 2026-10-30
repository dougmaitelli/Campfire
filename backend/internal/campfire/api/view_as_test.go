package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
	accessperm "campfire/internal/shared/permissions"
)

func previewAs(t *testing.T, admin http.Handler, userID int64) (*http.Cookie, http.Handler) {
	t.Helper()

	response := request(admin, "POST", "/api/v1/auth/view-as", fmt.Sprintf(`{"user_id":%d}`, userID))
	if response.Code != http.StatusNoContent {
		t.Fatal(response.Code, response.Body)
	}

	cookies := response.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "campfire_view_as" || !cookies[0].HttpOnly || cookies[0].Path != "/api" || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatal("preview cookie is not isolated from credentials", cookies)
	}

	return cookies[0], withPreviewCookie(admin, cookies[0])
}

func withPreviewCookie(handler http.Handler, cookie *http.Cookie) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.AddCookie(cookie)
		handler.ServeHTTP(w, r)
	})
}

func TestViewAsUsesUserPermissionsAndBlocksChanges(t *testing.T) {
	f := newAccessFixture(t)
	user, ordinary := f.user("preview-reader", database.UserRole)
	_, _, input := serverFixture(t, f.admin)
	response := request(f.admin, "POST", "/api/v1/servers", configJSON(t, input))

	var server model.Server
	if response.Code != http.StatusCreated || json.Unmarshal(response.Body.Bytes(), &server) != nil {
		t.Fatal(response.Code, response.Body)
	}

	response = request(f.admin, "POST", "/api/v1/roles", fmt.Sprintf(`{"name":"Preview reader","scope":"server","permissions":[%q]}`, accessperm.ServerFilesRead))

	var role service.RoleView
	if response.Code != http.StatusCreated || json.Unmarshal(response.Body.Bytes(), &role) != nil {
		t.Fatal(response.Code, response.Body)
	}

	endpoint := "/api/v1/servers/" + server.ID
	expectAccess(t, f.admin, "POST", endpoint+"/access/grants", fmt.Sprintf(`{"username":%q,"access_role_id":%d}`, user.Username, role.ID), http.StatusNoContent)
	expectAccess(t, ordinary, "POST", endpoint+"/actions", `{"action":"start"}`, http.StatusAccepted)

	_, preview := previewAs(t, f.admin, user.ID)
	response = request(preview, "GET", "/api/v1/auth/session", "")

	var state struct {
		service.Principal
		CSRF   string `json:"csrf_token"`
		ViewAs struct {
			AdminUserID int64 `json:"admin_user_id"`
		} `json:"view_as"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &state) != nil || state.UserID != user.ID || state.Admin || state.ViewAs.AdminUserID != 1 || state.CSRF == "" {
		t.Fatal("preview retained administrator authority", response.Code, response.Body)
	}

	expectAccess(t, preview, "GET", endpoint+"/files", "", http.StatusOK)
	expectAccess(t, preview, "POST", endpoint+"/files", `{"operation":"read","path":"config.txt"}`, http.StatusOK)
	expectAccess(t, preview, "GET", "/api/v1/users", "", http.StatusForbidden)
	expectAccess(t, preview, "GET", "/api/v1/tokens", "", http.StatusForbidden)

	before := f.runtime.calls.Load()

	for _, check := range []struct{ method, path, body string }{
		{"POST", endpoint + "/actions", `{"action":"start"}`},
		{"POST", endpoint + "/files", `{"operation":"write","path":"config.txt","data":"dGVzdA==","revision":"absent"}`},
		{"POST", endpoint + "/files", `{"operation":"delete","path":"config.txt"}`},
		{"PUT", endpoint, configJSON(t, input)},
		{"DELETE", endpoint, ""},
		{"POST", "/api/v1/tokens", `{"name":"Forbidden","scopes":["*"],"expires_in_days":1}`},
		{"POST", "/api/v1/auth/oidc/link", ""},
	} {
		expectAccess(t, preview, check.method, check.path, check.body, http.StatusForbidden)
	}

	if f.runtime.calls.Load() != before {
		t.Fatal("preview mutation reached Spark")
	}

	expectAccess(t, f.admin, "PUT", fmt.Sprintf("/api/v1/roles/%d", role.ID), fmt.Sprintf(`{"name":"Preview reader","scope":"server","permissions":[%q]}`, accessperm.ServerView), http.StatusOK)
	expectAccess(t, preview, "GET", endpoint+"/files", "", http.StatusForbidden)
	expectAccess(t, preview, "GET", endpoint, "", http.StatusOK)

	response = request(preview, "DELETE", "/api/v1/auth/view-as", "")
	if response.Code != http.StatusNoContent || len(response.Result().Cookies()) != 1 || response.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal("preview could not be cleared", response.Code, response.Body)
	}

	expectAccess(t, f.admin, "GET", "/api/v1/users", "", http.StatusOK)
	expectAccess(t, ordinary, "GET", "/api/v1/auth/session", "", http.StatusOK)
	expectAccess(t, preview, "POST", "/api/v1/auth/logout", "", http.StatusNoContent)
	expectAccess(t, f.admin, "GET", "/api/v1/servers", "", http.StatusUnauthorized)
	expectAccess(t, ordinary, "GET", endpoint, "", http.StatusOK)
}

func TestViewAsIsOnlyForAdminBrowserSessions(t *testing.T) {
	f := newAccessFixture(t)
	target, _ := f.user("preview-target", database.UserRole)

	for _, id := range []int64{database.UserRole, database.CreatorRole} {
		_, regular := f.user(fmt.Sprintf("preview-role-%d", id), id)
		expectAccess(t, regular, "POST", "/api/v1/auth/view-as", fmt.Sprintf(`{"user_id":%d}`, target.ID), http.StatusForbidden)
		expectAccess(t, regular, "DELETE", "/api/v1/auth/view-as", "", http.StatusForbidden)
	}

	_, token := createTestAPIToken(t, f.admin, `["*"]`)
	expectAccess(t, bearerAPI(f.panel, token), "POST", "/api/v1/auth/view-as", fmt.Sprintf(`{"user_id":%d}`, target.ID), http.StatusForbidden)
	expectAccess(t, f.admin, "POST", "/api/v1/auth/view-as", `{"user_id":1}`, http.StatusBadRequest)
	expectAccess(t, f.admin, "POST", "/api/v1/auth/view-as", `{"user_id":999999}`, http.StatusNotFound)

	cookie, _ := previewAs(t, f.admin, target.ID)
	_, ordinary := f.user("preview-cookie-holder", database.UserRole)

	response := request(withPreviewCookie(ordinary, cookie), "GET", "/api/v1/auth/session", "")
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), `"view_as"`) || strings.Contains(response.Body.String(), `"username":"preview-target"`) {
		t.Fatal("another user's cookie enabled preview for a non-admin", response.Code, response.Body)
	}

	forged := *cookie
	forged.Value = strings.Replace(cookie.Value, fmt.Sprint(target.ID), "999999", 1)

	response = request(withPreviewCookie(f.admin, &forged), "GET", "/api/v1/auth/session", "")
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), `"view_as"`) {
		t.Fatal("forged preview cookie was honored", response.Code, response.Body)
	}

	session, raw, err := f.auth.Login(t.Context(), "admin", testAdminPassword, "")
	if err != nil {
		t.Fatal(err)
	}

	newBrowser := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.AddCookie(&http.Cookie{Name: "campfire_session", Value: raw})
		r.Header.Set("X-CSRF-Token", session.CSRF)
		f.panel.ServeHTTP(w, r)
	})

	response = request(withPreviewCookie(newBrowser, cookie), "GET", "/api/v1/auth/session", "")
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), `"view_as"`) {
		t.Fatal("preview leaked into another admin session", response.Code, response.Body)
	}

	for _, csrf := range []string{"", "forged"} {
		response = authRequest(f.panel, "POST", "/api/v1/auth/view-as", fmt.Sprintf(`{"user_id":%d}`, target.ID), &http.Cookie{Name: "campfire_session", Value: raw}, csrf, "")
		if response.Code != http.StatusForbidden {
			t.Fatal("preview start bypassed CSRF", response.Code, response.Body)
		}
	}

	otherAdmin, _ := f.user("preview-other-admin", database.AdminRole)
	_, privilegedPreview := previewAs(t, f.admin, otherAdmin.ID)
	expectAccess(t, privilegedPreview, "GET", "/api/v1/users", "", http.StatusOK)
	expectAccess(t, privilegedPreview, "POST", "/api/v1/users", `{"username":"forbidden","password":"a long enough password","role_id":3}`, http.StatusForbidden)
}

func TestViewAsRechecksAccountAvailability(t *testing.T) {
	f := newAccessFixture(t)
	target, _ := f.user("preview-revoked", database.UserRole)
	_, preview := previewAs(t, f.admin, target.ID)
	expectAccess(t, f.admin, "PUT", fmt.Sprintf("/api/v1/users/%d", target.ID), `{"role_id":3,"disabled":true}`, http.StatusOK)
	expectAccess(t, preview, "GET", "/api/v1/servers", "", http.StatusBadRequest)

	response := request(preview, "GET", "/api/v1/auth/session", "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"view_as_ended":true`) || len(response.Result().Cookies()) != 1 || response.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal("unavailable preview did not return explicitly to Admin", response.Code, response.Body)
	}

	expectAccess(t, f.admin, "POST", "/api/v1/auth/view-as", fmt.Sprintf(`{"user_id":%d}`, target.ID), http.StatusBadRequest)
	expectAccess(t, preview, "DELETE", "/api/v1/auth/view-as", "", http.StatusNoContent)
}
