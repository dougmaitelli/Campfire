//go:build !production

// Campfire-mock runs the real panel against simulated development Sparks.
package main

import (
	"context"
	"log/slog"
	"os"

	"campfire/internal/devmock"
)

func main() {
	if err := devmock.Run(context.Background(), os.Args[1:]); err != nil {
		slog.Error("Mock Campfire stopped", "error", err)
		os.Exit(1)
	}
}
