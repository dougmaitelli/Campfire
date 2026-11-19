//go:build !production

package devmock

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"campfire/internal/campfire/cli"
	"campfire/internal/campfire/database"
	"campfire/internal/campfire/service"
	mockintegrations "campfire/internal/devmock/integrations"
	"campfire/internal/shared/settings"
)

// Keep all startup extensions compile-checked, not just the minimal dispatcher.
var (
	_ cli.RuntimeBackend         = (*Runtime)(nil)
	_ cli.EmberUpdateFetcher     = (*Runtime)(nil)
	_ cli.SparkVersionFetcher    = (*Runtime)(nil)
	_ cli.GameIntegrationStarter = (*Runtime)(nil)
)

// Run runs an isolated, loopback-only development panel with simulated Sparks.
func Run(ctx context.Context, args []string) error {
	return run(ctx, args, "data/mock", cli.RunWithOptions)
}

// The application owns its state and composition; tests can run this same path
// with disposable state and an isolated HTTP transport around the real panel.
func run(ctx context.Context, args []string, directory string, start func(context.Context, []string, cli.StartupOptions) error) error {
	if len(args) != 0 {
		return errors.New("campfire-mock takes no arguments; state is stored in data/mock")
	}

	// A developer's production settings must never reach this disposable panel.
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "CAMPFIRE_") {
			if err := os.Unsetenv(key); err != nil {
				return err
			}
		}
	}

	discord := mockintegrations.NewDiscord()
	defer discord.Close()

	return start(ctx, []string{"--" + settings.CampfireListen.FlagName, "127.0.0.1:8080", "--" + settings.CampfireDatabase.FlagName, filepath.Join(directory, "panel.db"), "--" + settings.CampfireData.FlagName, directory, "--" + settings.CampfireAgentListen.FlagName, ""}, cli.StartupOptions{Discord: discord.Client(), WrapHandler: discord.WrapHandler, Initialize: func(ctx context.Context, db *database.Database, auth *service.AuthService) (cli.RuntimeBackend, error) {
		runtime, err := New(ctx, db.ORM(), auth)
		if err == nil {
			err = runtime.EnableJobExamples(ctx)
		}

		if err == nil {
			err = runtime.EnableHistoryExamples(ctx)
		}

		if err == nil {
			err = runtime.EnableIntegrationExample(ctx, discord, filepath.Join(directory, "integration-secrets"))
		}

		if err == nil {
			slog.Info("MOCK MODE: simulated Sparks; no Docker, TLS or infrastructure connections", "database", filepath.Join(directory, "panel.db"))
			slog.Info("Mock username: " + Username)
			slog.Info("Mock password: " + Password)
			slog.Info("Mock panel: http://127.0.0.1:8081")
			discord.LogSetup()
		}

		return runtime, err
	}})
}
