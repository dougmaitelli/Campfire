//go:build !production

package cli

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/devmock"
	mockintegrations "campfire/internal/campfire/devmock/integrations"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/settings"
)

// RunMock runs an isolated, loopback-only development panel with simulated Sparks.
func RunMock(ctx context.Context, args []string) error {
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

	return run(ctx, []string{"--" + settings.CampfireListen.FlagName, "127.0.0.1:8080", "--" + settings.CampfireDatabase.FlagName, "data/mock/panel.db", "--" + settings.CampfireData.FlagName, "data/mock", "--" + settings.CampfireAgentListen.FlagName, ""}, startupOptions{discord: discord.Client(), wrapHandler: discord.WrapHandler, initialize: func(ctx context.Context, db *database.Database, auth *service.AuthService) (runtimeBackend, error) {
		runtime, err := devmock.New(ctx, db.ORM(), auth)
		if err == nil {
			err = runtime.EnableJobExamples(ctx)
		}

		if err == nil {
			err = runtime.EnableIntegrationExample(ctx, discord, "data/mock/integration-secrets")
		}

		if err == nil {
			slog.Info("MOCK MODE: simulated Sparks; no Docker, TLS or infrastructure connections", "database", "data/mock/panel.db")
			slog.Info("Mock username: " + devmock.Username)
			slog.Info("Mock password: " + devmock.Password)
			slog.Info("Mock panel: http://127.0.0.1:8081")
			discord.LogSetup()
		}

		return runtime, err
	}})
}
