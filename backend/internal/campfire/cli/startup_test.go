package cli

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"testing"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/settings"
)

func TestVersionDoesNotConstructInjectedRuntime(t *testing.T) {
	err := RunWithOptions(t.Context(), []string{"--" + settings.Version.FlagName}, StartupOptions{
		Initialize: func(context.Context, *database.Database, *service.AuthService) (RuntimeBackend, error) {
			t.Fatal("version constructed the runtime")
			return nil, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeInitializationFailureStopsStartup(t *testing.T) {
	failure := errors.New("runtime construction failed")
	called := false

	err := RunWithOptions(t.Context(), []string{
		"--" + settings.CampfireDatabase.FlagName, filepath.Join(t.TempDir(), "panel.db"),
		"--" + settings.CampfireListen.FlagName, "127.0.0.1:0",
		"--" + settings.CampfireAgentListen.FlagName, "",
	}, StartupOptions{
		Initialize: func(ctx context.Context, db *database.Database, auth *service.AuthService) (RuntimeBackend, error) {
			called = true

			if ctx.Err() != nil || db == nil || auth == nil {
				t.Fatal("runtime did not receive initialized panel dependencies")
			}

			return nil, failure
		},
		WrapHandler: func(http.Handler) http.Handler {
			t.Fatal("listener assembly continued after runtime initialization failed")
			return nil
		},
	})
	if !called || !errors.Is(err, failure) {
		t.Fatal("runtime construction error was lost", called, err)
	}
}
