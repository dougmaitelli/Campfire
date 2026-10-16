//go:build !production

// Campfire-mock runs the real panel against simulated development Sparks.
package main

import (
	"context"
	"log/slog"
	"os"

	"campfire/internal/campfire/cli"
)

func main() {
	if err := cli.RunMock(context.Background(), os.Args[1:]); err != nil {
		slog.Error("Mock Campfire stopped", "error", err)
		os.Exit(1)
	}
}
