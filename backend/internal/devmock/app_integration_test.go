//go:build !production

package devmock

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"campfire/internal/campfire/cli"
	"campfire/internal/campfire/database"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/settings"
)

type observedApplicationRuntime struct {
	*Runtime
	stopped chan struct{}
}

func (r *observedApplicationRuntime) StartGameIntegrations(ctx context.Context, games *service.GameIntegrationService) func() {
	stop := r.Runtime.StartGameIntegrations(ctx, games)

	return func() {
		stop()
		close(r.stopped)
	}
}

func TestApplicationCompositionAndShutdown(t *testing.T) {
	// Run clears all panel overrides. Restore the caller's environment afterward.
	for _, entry := range os.Environ() {
		key, value, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "CAMPFIRE_") {
			t.Setenv(key, value)
		}
	}

	directory := t.TempDir()
	forbidden := filepath.Join(directory, "production.db")
	t.Setenv(settings.CampfireDatabase.EnvironmentName, forbidden)
	t.Setenv(settings.CampfireOIDCIssuer.EnvironmentName, "invalid-issuer")
	t.Setenv(settings.CampfireDisableLocalLogin.EnvironmentName, "true")

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	ready := make(chan http.Handler, 1)
	finished := make(chan error, 1)
	stopped := make(chan struct{})

	var runtime *Runtime

	go func() {
		finished <- run(ctx, nil, directory, func(ctx context.Context, args []string, options cli.StartupOptions) error {
			// Keep the real panel lifecycle, but never contend for the dev port.
			for i, arg := range args {
				if arg == "--"+settings.CampfireListen.FlagName {
					args[i+1] = "127.0.0.1:0"
				}
			}

			initialize := options.Initialize
			options.Initialize = func(ctx context.Context, db *database.Database, auth *service.AuthService) (cli.RuntimeBackend, error) {
				backend, err := initialize(ctx, db, auth)
				if err != nil {
					return nil, err
				}

				var ok bool

				runtime, ok = backend.(*Runtime)
				if !ok {
					return nil, fmt.Errorf("unexpected application runtime %T", backend)
				}

				return &observedApplicationRuntime{Runtime: runtime, stopped: stopped}, nil
			}
			wrap := options.WrapHandler
			options.WrapHandler = func(handler http.Handler) http.Handler {
				handler = wrap(handler)
				ready <- handler

				return handler
			}

			return cli.RunWithOptions(ctx, args, options)
		})
	}()

	defer func() {
		cancel()

		select {
		case err := <-finished:
			if err != nil {
				t.Errorf("application shutdown: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("application did not stop")
		}

		select {
		case <-stopped:
		default:
			t.Error("application did not stop and join its game publishers")
		}
	}()

	var handler http.Handler
	select {
	case handler = <-ready:
	case <-ctx.Done():
		t.Fatal("application did not become ready", ctx.Err())
	}

	server := httptest.NewServer(handler)
	defer server.Close()

	client := server.Client()
	client.Timeout = 5 * time.Second

	var err error

	client.Jar, err = cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}

	request := func(method, path, body string) []byte {
		t.Helper()

		req, err := http.NewRequestWithContext(ctx, method, server.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", server.URL)

		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}

		defer func() { _ = response.Body.Close() }()

		data, err := io.ReadAll(response.Body)
		if err != nil || response.StatusCode != http.StatusOK {
			t.Fatalf("%s: status=%d body=%s error=%v", path, response.StatusCode, data, err)
		}

		return data
	}

	var login struct {
		Authenticated bool `json:"authenticated"`
	}

	body, err := json.Marshal(map[string]string{"username": Username, "password": Password})
	if err != nil {
		t.Fatal(err)
	}

	if err := json.Unmarshal(request(http.MethodPost, "/api/v1/auth/login", string(body)), &login); err != nil || !login.Authenticated {
		t.Fatal("seeded login failed", err)
	}

	var release service.SparkReleaseStatus

	if err := json.Unmarshal(request(http.MethodGet, "/api/v1/sparks/update-status", ""), &release); err != nil {
		t.Fatal(err)
	}

	version, err := runtime.LatestSparkVersion(ctx)
	if err != nil || release.Error != "" || release.LatestVersion != version {
		t.Fatal("runtime version source was not wired", release, err)
	}

	for _, path := range []string{"/api/v1/servers", "/api/v1/servers/survival/integrations/discord/channels", "/mock/discord", "/mock/discord/viewer.js", "/mock/discord/viewer.css", "/mock/discord/state"} {
		if len(request(http.MethodGet, path, "")) == 0 {
			t.Fatal("empty application response", path)
		}
	}

	if _, err := os.Stat(forbidden); !os.IsNotExist(err) {
		t.Fatal("production database override was used", err)
	}

	if _, err := os.Stat(filepath.Join(directory, "panel.db")); err != nil {
		t.Fatal("isolated database missing", err)
	}

	runtime.mu.Lock()
	started := runtime.games != nil
	runtime.mu.Unlock()

	if !started {
		t.Fatal("application did not connect game publishers to ingestion")
	}

	// Observe actual worker activity, not just seeded events or a direct unit call.
	for {
		runtime.mu.Lock()
		publishing := runtime.games != nil && len(runtime.gameSessions) > 0
		runtime.mu.Unlock()

		if publishing {
			break
		}

		select {
		case <-ctx.Done():
			t.Fatal("application did not start game publishers", ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
}
