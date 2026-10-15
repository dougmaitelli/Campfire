// Campfire is the central panel executable.
package main

import (
	"context"
	"log/slog"
	"os"

	"campfire/internal/campfire/cli"
)

func main() {
	if err := cli.Run(context.Background(), os.Args[1:]); err != nil {
		slog.Error("Campfire stopped", "error", err)
		os.Exit(1)
	}
}
