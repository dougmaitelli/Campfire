package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"
)

func createTestAPIToken(t *testing.T, handler http.Handler, scopes string) (database.APIToken, string) {
	t.Helper()

	response := request(handler, "POST", "/api/v1/tokens", `{"name":"integration","expires_in_days":30,"scopes":`+scopes+`}`)
	if response.Code != http.StatusCreated {
		t.Fatal(response.Code, response.Body)
	}

	var result struct {
		Token  database.APIToken `json:"token"`
		Secret string            `json:"secret"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}

	if result.Secret == "" {
		t.Fatal("missing token secret")
	}

	return result.Token, result.Secret
}

func bearerAPI(handler http.Handler, secret string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+secret)
		handler.ServeHTTP(w, r)
	})
}

func TestAPITokensInheritPermissionsAndRestrictAdmin(t *testing.T) {
	f := newAccessFixture(t)
	user, browser := f.user("owner", database.UserRole)
	_, _, input := serverFixture(t, f.admin)

	response := request(f.admin, "POST", "/api/v1/servers", configJSON(t, input))
	if response.Code != http.StatusCreated {
		t.Fatal(response.Code, response.Body)
	}

	var server model.Server
	if err := json.Unmarshal(response.Body.Bytes(), &server); err != nil {
		t.Fatal(err)
	}

	endpoint := "/api/v1/servers/" + server.ID
	token, secret := createTestAPIToken(t, browser, `["*"]`)
	bearer := bearerAPI(f.panel, secret)
	expectAccess(t, bearer, "GET", endpoint, "", http.StatusNotFound)
	expectAccess(t, f.admin, "PUT", endpoint+"/owner", fmt.Sprintf(`{"user_id":%d}`, user.ID), http.StatusOK)
	expectAccess(t, bearer, "GET", endpoint, "", http.StatusOK)
	expectAccess(t, bearer, "GET", "/api/v1/sparks", "", http.StatusForbidden)
	expectAccess(t, bearer, "GET", "/api/v1/users", "", http.StatusForbidden)
	_, elevated := createTestAPIToken(t, browser, `["users.manage"]`)
	expectAccess(t, bearerAPI(f.panel, elevated), "GET", "/api/v1/users", "", http.StatusForbidden)

	for _, path := range []string{"/api/v1/tokens", "/api/v1/auth/session"} {
		expectAccess(t, bearer, "GET", path, "", http.StatusForbidden)
	}

	expectAccess(t, bearer, "POST", "/api/v1/tokens", `{}`, http.StatusForbidden)
	expectAccess(t, bearer, "DELETE", fmt.Sprintf("/api/v1/tokens/%d", token.ID), "", http.StatusForbidden)

	listing := request(browser, "GET", "/api/v1/tokens", "")
	if listing.Code != http.StatusOK || strings.Contains(listing.Body.String(), secret) || strings.Contains(listing.Body.String(), `"hash"`) {
		t.Fatal("secret leaked", listing.Code, listing.Body)
	}

	_, other := f.user("other", database.UserRole)
	expectAccess(t, other, "DELETE", fmt.Sprintf("/api/v1/tokens/%d", token.ID), "", http.StatusNotFound)
	_, scoped := createTestAPIToken(t, f.admin, `["server.view"]`)
	limited := bearerAPI(f.panel, scoped)
	expectAccess(t, limited, "GET", endpoint, "", http.StatusOK)
	expectAccess(t, limited, "GET", "/api/v1/sparks", "", http.StatusForbidden)
	expectAccess(t, limited, "DELETE", endpoint, "", http.StatusForbidden)
	expectAccess(t, limited, "POST", endpoint+"/actions", `{"action":"start"}`, http.StatusForbidden)
	expectAccess(t, f.admin, "PUT", endpoint+"/owner", `{"user_id":1}`, http.StatusOK)
	expectAccess(t, bearer, "GET", endpoint, "", http.StatusNotFound)
	expectAccess(t, browser, "DELETE", fmt.Sprintf("/api/v1/tokens/%d", token.ID), "", http.StatusNoContent)
	expectAccess(t, bearer, "GET", "/api/v1/servers", "", http.StatusUnauthorized)
}

func TestAPITokenInvalidHeadersNeverFallBackToSession(t *testing.T) {
	f := newAccessFixture(t)

	_, secret := createTestAPIToken(t, f.admin, `["*"]`)
	for _, headers := range [][]string{{""}, {"Basic invalid"}, {"Bearer cf_invalid"}, {"Bearer " + secret, "Bearer " + secret}} {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Header["Authorization"] = headers
			f.admin.ServeHTTP(w, r)
		})
		expectAccess(t, handler, "GET", "/api/v1/servers", "", http.StatusUnauthorized)
	}

	user, browser := f.user("disabled", database.UserRole)
	_, userSecret := createTestAPIToken(t, browser, `["*"]`)
	expectAccess(t, bearerAPI(f.panel, userSecret), "GET", "/api/v1/servers", "", http.StatusOK)
	expectAccess(t, f.admin, "PUT", fmt.Sprintf("/api/v1/users/%d", user.ID), `{"role_id":3,"disabled":true}`, http.StatusOK)
	expectAccess(t, bearerAPI(f.panel, userSecret), "GET", "/api/v1/servers", "", http.StatusUnauthorized)
}

func TestAPITokenBackupDownloadStopsAfterRevocation(t *testing.T) {
	f := newAccessFixture(t)
	_, browser := f.user("downloader", database.UserRole)
	_, _, input := serverFixture(t, f.admin)
	response := request(f.admin, "POST", "/api/v1/servers", configJSON(t, input))

	var server model.Server
	if err := json.Unmarshal(response.Body.Bytes(), &server); err != nil {
		t.Fatal(err)
	}

	endpoint := "/api/v1/servers/" + server.ID
	expectAccess(t, f.admin, "POST", endpoint+"/members", `{"username":"downloader"}`, http.StatusOK)
	token, secret := createTestAPIToken(t, browser, `["server.view","server.operate"]`)
	f.runtime.files = func(input model.FileRequest) (model.FileResult, error) {
		if input.Offset > 0 {
			expectAccess(t, browser, "DELETE", fmt.Sprintf("/api/v1/tokens/%d", token.ID), "", http.StatusNoContent)
		}

		return model.FileResult{ServerID: input.ServerID, Data: []byte{1}, Size: 2}, nil
	}

	response = request(bearerAPI(f.panel, secret), "GET", endpoint+"/backups/game.tar.gz", "")
	if response.Header().Get("Content-Length") != "2" || response.Body.Len() != 1 {
		t.Fatal("download continued after token revocation", response.Body.Len())
	}
}
