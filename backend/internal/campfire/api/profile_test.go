package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"campfire/internal/campfire/database"
)

func TestOwnProfileCannotChangePrivilegesOrAnotherUser(t *testing.T) {
	f := newAccessFixture(t)
	user, browser := f.user("profile-owner", database.UserRole)
	response := request(browser, "GET", "/api/v1/auth/profile", "")
	var profile struct {
		User          database.User `json:"user"`
		LocalPassword bool          `json:"local_password"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &profile) != nil || profile.User.ID != user.ID || !profile.LocalPassword {
		t.Fatal(response.Code, response.Body)
	}
	for _, secret := range []string{"password_hash", "credential_version", "csrf", "secret_ref"} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatal("profile leaked private data", secret)
		}
	}
	response = request(browser, "PUT", "/api/v1/auth/profile", `{"name":"My name","email":"me@example.com","role_id":1,"disabled":false,"user_id":1}`)
	if response.Code != http.StatusBadRequest {
		t.Fatal("privilege fields accepted", response.Code, response.Body)
	}
	response = request(browser, "PUT", "/api/v1/auth/profile", `{"name":"My name","email":"me@example.com"}`)
	if response.Code != http.StatusOK {
		t.Fatal(response.Code, response.Body)
	}
	var saved database.User
	err := f.db.ORM().Take(&saved, user.ID).Error
	if err != nil || saved.Name != "My name" || saved.Email != "me@example.com" || saved.RoleID != database.UserRole {
		t.Fatal(saved, err)
	}
	_, preview := previewAs(t, f.admin, user.ID)
	expectAccess(t, preview, "GET", "/api/v1/auth/profile", "", http.StatusForbidden)
	expectAccess(t, preview, "PUT", "/api/v1/auth/profile", `{}`, http.StatusForbidden)
	_, secret := createTestAPIToken(t, browser, `["*"]`)
	expectAccess(t, bearerAPI(f.panel, secret), "GET", "/api/v1/auth/profile", "", http.StatusForbidden)
	expectAccess(t, bearerAPI(f.panel, secret), "PUT", "/api/v1/auth/profile", `{}`, http.StatusForbidden)
}
