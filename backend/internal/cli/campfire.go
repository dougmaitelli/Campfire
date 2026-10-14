// Package cli provides command-line parsing and application startup.
package cli

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"campfire/internal/api"
	"campfire/internal/database"
	"campfire/internal/database/repository"
	"campfire/internal/service"
	"campfire/internal/spark"
)

// Run starts the admin API and optional dedicated agent TLS listener.
func Run(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("campfire", flag.ContinueOnError)
	listen := flags.String("listen", "127.0.0.1:8080", "admin API listen address")
	dbPath := flags.String("database", "data/panel.db", "SQLite database path")
	agentListen := flags.String("agent-listen", "", "enable the dedicated agent HTTPS listener, e.g. :8443")
	tlsCert := flags.String("agent-tls-cert", "", "agent listener HTTPS certificate file")

	tlsKey := flags.String("agent-tls-key", "", "agent listener HTTPS private key file")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}

		return err
	}

	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}

	if *agentListen != "" {
		host, _, err := net.SplitHostPort(*listen)

		ip, parseErr := netip.ParseAddr(host)
		if err != nil || (host != "localhost" && (parseErr != nil || !ip.IsLoopback())) {
			return errors.New("remote enrollment requires a loopback admin listener until panel authentication is implemented")
		}

		if *tlsCert == "" || *tlsKey == "" {
			return errors.New("--agent-listen requires --agent-tls-cert and --agent-tls-key")
		}
	}

	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	db, err := database.Open(ctx, *dbPath)
	if err != nil {
		return err
	}

	defer func() { _ = db.Close() }()

	var hub *spark.Hub

	var connectivity service.SparkConnectivity

	var agentTLS *tls.Config

	if *agentListen != "" {
		certificate, err := tls.LoadX509KeyPair(*tlsCert, *tlsKey)
		if err != nil {
			return fmt.Errorf("load agent HTTPS certificate: %w", err)
		}

		authority, err := spark.LoadAuthority(filepath.Join(filepath.Dir(*dbPath), "spark-authority.json"))
		if err != nil {
			return fmt.Errorf("load agent authority: %w", err)
		}

		hub = spark.NewHub(repository.NewEnrollmentRepository(db.ORM()), authority)
		defer hub.Close()

		connectivity = hub
		agentTLS = hub.TLSConfig()
		agentTLS.Certificates = []tls.Certificate{certificate}
	}

	embers := service.NewEmberService(repository.NewEmberRepository(db.ORM()))
	servers := service.NewServerService(repository.NewServerRepository(db.ORM()), embers)
	handler := api.New(service.NewSparkService(repository.NewSparkRepository(db.ORM()), connectivity), embers, servers).WithAgentHub(hub).Handler()
	admin := newHTTPServer(ctx, handler)

	var active []*http.Server

	var listeners []net.Listener

	defer func() {
		for _, server := range active {
			_ = server.Close()
		}

		for _, listener := range listeners {
			_ = listener.Close()
		}
	}()

	listenConfig := net.ListenConfig{}

	listener, err := listenConfig.Listen(ctx, "tcp", *listen)
	if err != nil {
		return fmt.Errorf("listen for admin API: %w", err)
	}

	listeners = append(listeners, listener)
	active = append(active, admin)

	if hub != nil {
		listener, err := listenConfig.Listen(ctx, "tcp", *agentListen)
		if err != nil {
			return fmt.Errorf("listen for agents: %w", err)
		}

		listeners = append(listeners, tls.NewListener(listener, agentTLS))
		active = append(active, newHTTPServer(ctx, hub.Handler()))
	}

	stopped := make(chan error, len(active))
	for i, server := range active {
		go func() { stopped <- server.Serve(listeners[i]) }()

		slog.Info("Campfire listener started", "address", listeners[i].Addr(), "agents", i > 0)
	}

	select {
	case err := <-stopped:
		return fmt.Errorf("campfire listener stopped: %w", err)
	case <-ctx.Done():
		if hub != nil {
			hub.Close()
		}

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()

		for _, server := range active {
			if err := server.Shutdown(shutdownCtx); err != nil {
				_ = server.Close()
				return fmt.Errorf("shutdown API: %w", err)
			}
		}

		return nil
	}
}

func newHTTPServer(ctx context.Context, handler http.Handler) *http.Server {
	return &http.Server{
		Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
		WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second,
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
}
