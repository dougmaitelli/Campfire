// Package cli provides command-line parsing and application startup.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"campfire/internal/api"
	"campfire/internal/database"
	"campfire/internal/database/repository"
	"campfire/internal/service"
)

// Run starts the panel API and shuts it down on cancellation or a termination signal.
func Run(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("campfire", flag.ContinueOnError)
	listen := flags.String("listen", "127.0.0.1:8080", "API listen address")

	dbPath := flags.String("database", "data/panel.db", "SQLite database path")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}

		return err
	}

	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}

	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	db, err := database.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	handler := api.New(service.NewSparkService(repository.NewSparkRepository(db.ORM()), nil)).Handler()
	server := &http.Server{
		Addr: *listen, Handler: handler,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
		WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second,
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	listenConfig := net.ListenConfig{}

	listener, err := listenConfig.Listen(ctx, "tcp", *listen)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	stopped := make(chan error, 1)
	go func() { stopped <- server.Serve(listener) }()

	slog.Info("Campfire API listening", "address", listener.Addr())

	select {
	case err := <-stopped:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}

		return fmt.Errorf("serve API: %w", err)
	case <-ctx.Done():
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return fmt.Errorf("shutdown API: %w", err)
		}

		return nil
	}
}
