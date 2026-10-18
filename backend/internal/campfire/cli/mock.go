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
	"campfire/internal/campfire/service"
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

	return run(ctx, []string{"--listen", "127.0.0.1:8080", "--database", "data/mock/panel.db", "--data-dir", "data/mock", "--agent-listen", ""}, func(ctx context.Context, db *database.Database, auth *service.AuthService) (runtimeBackend, error) {
		runtime, err := devmock.New(ctx, db.ORM(), auth)
		if err == nil {
			slog.Info("MOCK MODE: simulated Sparks; no Docker, TLS or infrastructure connections", "database", "data/mock/panel.db", "username", devmock.Username, "password", devmock.Password)
		}

		return runtime, err
	})
}
