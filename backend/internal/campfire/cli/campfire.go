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
	filesftp "campfire/internal/campfire/sftp"
	"campfire/internal/campfire/webui"
)

// Run starts the admin API and the dedicated agent TLS listener.
func Run(ctx context.Context, args []string) error {
	return run(ctx, args, nil)
}

type runtimeBackend interface {
	service.SparkConnectivity
	service.ConfigurationDispatcher
}

type runtimeInitializer func(context.Context, *database.Database, *service.AuthService) (runtimeBackend, error)

func run(ctx context.Context, args []string, initialize runtimeInitializer) error {
	if len(args) > 0 && args[0] == "reset-password" {
		return resetPassword(ctx, args[1:])
	}

	flags := flag.NewFlagSet("campfire", flag.ContinueOnError)

	disableLocalDefault, err := strconv.ParseBool(environment("CAMPFIRE_DISABLE_LOCAL_LOGIN", "false"))
	if err != nil {
		return errors.New("CAMPFIRE_DISABLE_LOCAL_LOGIN must be a boolean")
	}

	listen := flags.String("listen", environment("CAMPFIRE_LISTEN", "127.0.0.1:8080"), "browser and API listen address (CAMPFIRE_LISTEN)")
	dbSource := flags.String("database", environment("CAMPFIRE_DATABASE", "data/panel.db"), "SQLite path or PostgreSQL URL (CAMPFIRE_DATABASE)")
	dataDir := flags.String("data-dir", environment("CAMPFIRE_DATA", ""), "persistent state directory; defaults beside SQLite or ./data for PostgreSQL (CAMPFIRE_DATA)")
	sftpListen := flags.String("sftp-listen", environment("CAMPFIRE_SFTP_LISTEN", ""), "optional SFTP listen address (CAMPFIRE_SFTP_LISTEN)")
	sftpKey := flags.String("sftp-host-key", environment("CAMPFIRE_SFTP_HOST_KEY", ""), "persistent SFTP SSH host key (CAMPFIRE_SFTP_HOST_KEY)")
	maxUpload := flags.String("max-upload-mib", environment("CAMPFIRE_MAX_UPLOAD_MIB", "1024"), "maximum upload size in MiB (CAMPFIRE_MAX_UPLOAD_MIB)")
	publicURL := flags.String("public-url", environment("CAMPFIRE_PUBLIC_URL", ""), "browser origin for HTTPS reverse proxies (CAMPFIRE_PUBLIC_URL)")
	oidcIssuer := flags.String("oidc-issuer", environment("CAMPFIRE_OIDC_ISSUER", ""), "optional OIDC issuer URL (CAMPFIRE_OIDC_ISSUER)")
	oidcClient := flags.String("oidc-client-id", environment("CAMPFIRE_OIDC_CLIENT_ID", ""), "OIDC client ID (CAMPFIRE_OIDC_CLIENT_ID)")
	oidcSecret := flags.String("oidc-client-secret", environment("CAMPFIRE_OIDC_CLIENT_SECRET", ""), "OIDC client secret (CAMPFIRE_OIDC_CLIENT_SECRET)")
	disableLocal := flags.Bool("disable-local-login", disableLocalDefault, "require OIDC instead of passwords (CAMPFIRE_DISABLE_LOCAL_LOGIN)")
	agentListen := flags.String("agent-listen", environment("CAMPFIRE_AGENT_LISTEN", defaultAgentListen), "dedicated agent HTTPS listener; empty disables it (CAMPFIRE_AGENT_LISTEN)")
	agentPublicURL := flags.String("agent-public-url", environment("CAMPFIRE_AGENT_PUBLIC_URL", ""), "reachable agent HTTPS origin; defaults to the panel hostname and agent port (CAMPFIRE_AGENT_PUBLIC_URL)")
	tlsCert := flags.String("agent-tls-cert", environment("CAMPFIRE_AGENT_TLS_CERT", ""), "agent HTTPS certificate (CAMPFIRE_AGENT_TLS_CERT)")
	tlsKey := flags.String("agent-tls-key", environment("CAMPFIRE_AGENT_TLS_KEY", ""), "agent HTTPS private key (CAMPFIRE_AGENT_TLS_KEY)")
	version := flags.Bool("version", false, "print release version and exit")
	auditRetention := flags.String("audit-retention-days", environment("CAMPFIRE_AUDIT_RETENTION_DAYS", "0"), "audit record age limit in days; 0 keeps forever (CAMPFIRE_AUDIT_RETENTION_DAYS)")
	operationRetention := flags.String("operation-retention-days", environment("CAMPFIRE_OPERATION_RETENTION_DAYS", "0"), "completed operation history age limit in days; 0 keeps forever (CAMPFIRE_OPERATION_RETENTION_DAYS)")

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

	maxUploadBytes, err := parseMaxUpload(*maxUpload)
	if err != nil {
		return err
	}

	retention, err := parseHistoryRetention(*auditRetention, *operationRetention)
	if err != nil {
		return err
	}

	alerts, apprise, err := notificationConfig()
	if err != nil {
		return err
	}

	origin, err := adminOrigin(*listen, *publicURL)
	if err != nil {
		return err
	}

	endpoint, err := agentPublicAddress(*agentListen, *agentPublicURL, *publicURL)
	if err != nil {
		return err
	}

	if (*tlsCert == "") != (*tlsKey == "") {
		return errors.New("provide both --agent-tls-cert and --agent-tls-key, or omit both for generated agent TLS")
	}

	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	db, err := database.Open(ctx, *dbSource)
	if err != nil {
		return err
	}

	defer func() { _ = db.Close() }()

	stateDir := stateDirectory(*dbSource, *dataDir)

	auth := service.NewAuthService(repository.NewAuthRepository(db.ORM()))

	var runtime runtimeBackend

	if initialize != nil {
		runtime, err = initialize(ctx, db, auth)
		if err != nil {
			return fmt.Errorf("initialize development runtime: %w", err)
		}
	}

	provider, err := service.NewOIDCService(ctx, auth, service.OIDCConfig{Issuer: *oidcIssuer, ClientID: *oidcClient, ClientSecret: *oidcSecret, PublicURL: *publicURL, DisableLocal: *disableLocal})
	if err != nil {
		return err
	}

	setupToken, err := auth.PrepareSetup(ctx)
	if err != nil {
		return fmt.Errorf("prepare administrator setup: %w", err)
	}

	if setupToken != "" {
		slog.Info("Administrator setup token: " + setupToken)
	}

	var hub *agents.Hub

	var connectivity service.SparkConnectivity
	if runtime != nil {
		connectivity = runtime
	}

	var agentTLS *tls.Config

	if *agentListen != "" {
		authority, err := agents.LoadAuthority(filepath.Join(stateDir, "spark-authority.json"))
		if err != nil {
			return fmt.Errorf("load agent authority: %w", err)
		}

		hub = agents.NewHub(repository.NewEnrollmentRepository(db.ORM()), authority)
		defer hub.Close()

		connectivity = hub
		agentTLS = hub.TLSConfig()

		var serverCA func() []byte

		if *tlsCert != "" {
			certificate, err := tls.LoadX509KeyPair(*tlsCert, *tlsKey)
			if err != nil {
				return fmt.Errorf("load agent HTTPS certificate: %w", err)
			}

			agentTLS.Certificates = []tls.Certificate{certificate}
		} else {
			server, err := agents.LoadServerTLS(stateDir, agentCertificateHosts(endpoint))
			if err != nil {
				return fmt.Errorf("load generated agent HTTPS certificate: %w", err)
			}

			agentTLS.GetCertificate = server.GetCertificate
			serverCA = server.PEM
		}

		hub.WithEnrollmentEndpoint(endpoint, serverCA)
	}

	embers := service.NewEmberService(repository.NewEmberRepository(db.ORM()))

	embers.Updates = service.NewEmberUpdateService(embers, repository.NewEmberUpdateRepository(db.ORM()))
	if mock, ok := runtime.(interface {
		FetchEmberUpdate(context.Context, string, service.ImportOptions) (service.EmberPreview, error)
	}); ok {
		embers.Updates.WithFetcher(mock.FetchEmberUpdate)
	}

	embers.Updates.Start(ctx)
	defer embers.Updates.Close()

	servers := service.NewServerService(repository.NewServerRepository(db.ORM()), embers)
	sparks := service.NewSparkService(repository.NewSparkRepository(db.ORM()), connectivity)

	var sparkUpdater service.SparkUpdater
	if hub != nil {
		sparkUpdater = hub
	} else if updater, ok := runtime.(service.SparkUpdater); ok {
		sparkUpdater = updater
	}

	sparks.Updates = service.NewSparkUpdateService(sparks, sparkUpdater)
	if mock, ok := runtime.(interface {
		LatestSparkVersion(context.Context) (string, error)
	}); ok {
		sparks.Updates.WithFetcher(mock.LatestSparkVersion)
	}

	sparks.Updates.Start(ctx)
	defer sparks.Updates.Close()

	deployment := &service.DeploymentService{Servers: servers, Sparks: sparks, Embers: embers, MaxUploadBytes: maxUploadBytes}
	transfers := service.NewTransferService(repository.NewTransferRepository(db.ORM()), deployment)
	deployment.Transfers = transfers

	if hub != nil {
		deployment.Remote = hub
	} else if runtime != nil {
		deployment.Remote = runtime
	}

	transfers.Start(ctx)
	defer transfers.Close()

	activity := service.NewActivityService(repository.NewActivityRepository(db.ORM()), deployment, auth)
	activity.Retention = retention

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

	notifications := service.NewNotificationService(repository.NewNotificationRepository(db.ORM()), deployment, auth)

	notifications.Config, notifications.Apprise = alerts, apprise
	if err := notifications.Start(ctx); err != nil {
		return fmt.Errorf("start notifications: %w", err)
	}
	defer notifications.Close()

	handler := api.New(sparks, embers, servers).WithAgentHub(hub).WithDeployment(deployment).WithAuthentication(auth, origin).WithOIDC(provider).WithActivity(activity).WithNotifications(notifications).Handler()
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

	stopped := make(chan error, len(active)+1)

	if *sftpListen != "" {
		keyPath := *sftpKey
		if keyPath == "" {
			keyPath = filepath.Join(stateDir, "sftp_host_key")
		}

		key, err := filesftp.HostKey(keyPath)
		if err != nil {
			return fmt.Errorf("load SFTP host identity: %w", err)
		}

		listener, err := listenConfig.Listen(ctx, "tcp", *sftpListen)
		if err != nil {
			return fmt.Errorf("listen for SFTP: %w", err)
		}

		defer func() { _ = listener.Close() }()

		gateway := &filesftp.Server{Auth: auth, Deployment: deployment, Audit: activity.Store, HostKey: key}
		go func() { stopped <- gateway.Serve(ctx, listener) }()

		slog.Info("Campfire SFTP listener started", "address", listener.Addr())
	}

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
