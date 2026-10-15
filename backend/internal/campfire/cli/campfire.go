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
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"campfire/internal/campfire/agents"
	"campfire/internal/campfire/api"
	"campfire/internal/campfire/backupstorage"
	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"campfire/internal/campfire/webui"
)

// Run starts the admin API and optional dedicated agent TLS listener.
func Run(ctx context.Context, args []string) error {
	if len(args) > 0 && args[0] == "reset-password" {
		return resetPassword(ctx, args[1:])
	}

	flags := flag.NewFlagSet("campfire", flag.ContinueOnError)

	disableLocalDefault, err := strconv.ParseBool(environment("CAMPFIRE_DISABLE_LOCAL_LOGIN", "false"))
	if err != nil {
		return errors.New("CAMPFIRE_DISABLE_LOCAL_LOGIN must be a boolean")
	}

	listen := flags.String("listen", environment("CAMPFIRE_LISTEN", "127.0.0.1:8080"), "browser and API listen address (CAMPFIRE_LISTEN)")
	dbPath := flags.String("database", environment("CAMPFIRE_DATABASE", "data/panel.db"), "SQLite database path (CAMPFIRE_DATABASE)")
	publicURL := flags.String("public-url", environment("CAMPFIRE_PUBLIC_URL", ""), "browser origin for HTTPS reverse proxies (CAMPFIRE_PUBLIC_URL)")
	oidcIssuer := flags.String("oidc-issuer", environment("CAMPFIRE_OIDC_ISSUER", ""), "optional OIDC issuer URL (CAMPFIRE_OIDC_ISSUER)")
	oidcClient := flags.String("oidc-client-id", environment("CAMPFIRE_OIDC_CLIENT_ID", ""), "OIDC client ID (CAMPFIRE_OIDC_CLIENT_ID)")
	oidcSecret := flags.String("oidc-client-secret", environment("CAMPFIRE_OIDC_CLIENT_SECRET", ""), "OIDC client secret (CAMPFIRE_OIDC_CLIENT_SECRET)")
	disableLocal := flags.Bool("disable-local-login", disableLocalDefault, "require OIDC instead of passwords (CAMPFIRE_DISABLE_LOCAL_LOGIN)")
	agentListen := flags.String("agent-listen", environment("CAMPFIRE_AGENT_LISTEN", ""), "dedicated agent HTTPS listener (CAMPFIRE_AGENT_LISTEN)")
	tlsCert := flags.String("agent-tls-cert", environment("CAMPFIRE_AGENT_TLS_CERT", ""), "agent HTTPS certificate (CAMPFIRE_AGENT_TLS_CERT)")
	tlsKey := flags.String("agent-tls-key", environment("CAMPFIRE_AGENT_TLS_KEY", ""), "agent HTTPS private key (CAMPFIRE_AGENT_TLS_KEY)")
	version := flags.Bool("version", false, "print release version and exit")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}

		return err
	}

	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}

	if *version {
		return printVersion("Campfire")
	}

	origin, err := adminOrigin(*listen, *publicURL)
	if err != nil {
		return err
	}

	if *agentListen != "" {
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

	auth := service.NewAuthService(repository.NewAuthRepository(db.ORM()))

	provider, err := service.NewOIDCService(ctx, auth, service.OIDCConfig{Issuer: *oidcIssuer, ClientID: *oidcClient, ClientSecret: *oidcSecret, PublicURL: *publicURL, DisableLocal: *disableLocal})
	if err != nil {
		return err
	}

	setupToken, err := auth.PrepareSetup(ctx)
	if err != nil {
		return fmt.Errorf("prepare administrator setup: %w", err)
	}

	if setupToken != "" {
		slog.Info("Administrator setup required; enter this one-time token in the setup form", "setup_token", setupToken)
	}

	var hub *agents.Hub

	var connectivity service.SparkConnectivity

	var agentTLS *tls.Config

	if *agentListen != "" {
		certificate, err := tls.LoadX509KeyPair(*tlsCert, *tlsKey)
		if err != nil {
			return fmt.Errorf("load agent HTTPS certificate: %w", err)
		}

		authority, err := agents.LoadAuthority(filepath.Join(filepath.Dir(*dbPath), "spark-authority.json"))
		if err != nil {
			return fmt.Errorf("load agent authority: %w", err)
		}

		hub = agents.NewHub(repository.NewEnrollmentRepository(db.ORM()), authority)
		defer hub.Close()

		connectivity = hub
		agentTLS = hub.TLSConfig()
		agentTLS.Certificates = []tls.Certificate{certificate}
	}

	embers := service.NewEmberService(repository.NewEmberRepository(db.ORM()))
	servers := service.NewServerService(repository.NewServerRepository(db.ORM()), embers)
	sparks := service.NewSparkService(repository.NewSparkRepository(db.ORM()), connectivity)

	deployment := &service.DeploymentService{Servers: servers, Sparks: sparks, Embers: embers}
	transfers := service.NewTransferService(repository.NewTransferRepository(db.ORM()), deployment)
	deployment.Transfers = transfers

	if hub != nil {
		deployment.Remote = hub
	}

	transfers.Start(ctx)
	defer transfers.Close()

	activity := service.NewActivityService(repository.NewActivityRepository(db.ORM()), deployment, auth)

	deployment.Activity = activity
	if err := activity.Start(ctx); err != nil {
		return fmt.Errorf("start schedules: %w", err)
	}
	defer activity.Close()

	pathStyle, err := strconv.ParseBool(environment("CAMPFIRE_S3_PATH_STYLE", "false"))
	if err != nil {
		return errors.New("CAMPFIRE_S3_PATH_STYLE must be a boolean")
	}

	objectStore, err := backupstorage.New(ctx, backupstorage.Config{Bucket: environment("CAMPFIRE_S3_BUCKET", ""), Region: environment("CAMPFIRE_S3_REGION", "us-east-1"), Endpoint: environment("CAMPFIRE_S3_ENDPOINT", ""), Prefix: environment("CAMPFIRE_S3_PREFIX", ""), PathStyle: pathStyle})
	if err != nil {
		return fmt.Errorf("configure backup storage: %w", err)
	}

	var remoteBackups service.BackupStorage
	if objectStore != nil {
		remoteBackups = objectStore
	}

	backups := service.NewBackupService(repository.NewBackupRepository(db.ORM()), deployment, remoteBackups)
	deployment.Backups = backups

	backups.Start(ctx)
	defer backups.Close()

	handler := api.New(sparks, embers, servers).WithAgentHub(hub).WithDeployment(deployment).WithAuthentication(auth, origin).WithOIDC(provider).WithActivity(activity).Handler()
	admin := newHTTPServer(ctx, webui.Handler(handler))

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
		WriteTimeout: 40 * time.Second, IdleTimeout: 60 * time.Second,
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
}
