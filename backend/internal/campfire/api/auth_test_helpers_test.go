package api_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"campfire/internal/campfire/api"
	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
)

const testAdminPassword = "a long test administrator password"

// Existing feature tests use real authentication while focusing on their domain behavior.
func authenticatedTestAPI(t *testing.T, db *database.Database, panel *api.API) http.Handler {
	t.Helper()

	ctx := context.Background()
	auth := service.NewAuthService(repository.NewAuthRepository(db.ORM()))

	path := filepath.Join(t.TempDir(), "setup-token")
	if err := auth.PrepareSetup(ctx, path); err != nil {
		t.Fatal(err)
	}

	setup, err := auth.NeedsSetup(ctx)
	if err != nil {
		t.Fatal(err)
	}

	var session database.AdminSession

	var token string

	if setup {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		session, token, err = auth.Setup(ctx, strings.TrimSpace(string(data)), "admin", testAdminPassword)
		if err != nil {
			t.Fatal(err)
		}
	} else {
		session, token, err = auth.Login(ctx, "admin", testAdminPassword, "")
		if err != nil {
			t.Fatal(err)
		}
	}

	handler := panel.WithAuthentication(auth, "").Handler()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.AddCookie(&http.Cookie{Name: "campfire_session", Value: token})
		r.Header.Set("X-CSRF-Token", session.CSRF)
		handler.ServeHTTP(w, r)
	})
}
