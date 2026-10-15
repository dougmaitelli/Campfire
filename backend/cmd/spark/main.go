// Spark is the remote node agent executable.
package main

import (
	"campfire/internal/sparkcli"
	"context"
	"log/slog"
	"os"
)

func main() {
	if err := sparkcli.RunSpark(context.Background(), os.Args[1:]); err != nil {
		slog.Error("Spark stopped", "error", err)
		os.Exit(1)
	}
}
