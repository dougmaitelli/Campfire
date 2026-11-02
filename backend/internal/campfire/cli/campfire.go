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
	"campfire/internal/shared/cliutil"
	"campfire/internal/shared/limits"
	"campfire/internal/shared/settings"
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

	disableLocal, err := settings.CampfireDisableLocalLogin.Bool(flags, false, "require OIDC instead of passwords")
	if err != nil {
		return err
	}

	autoLinkEmail, err := settings.CampfireOIDCAutoLinkEmail.Bool(flags, false, "link first-time OIDC logins to local accounts by verified email")
	if err != nil {
		return err
	}

	listen := settings.CampfireListen.String(flags, "127.0.0.1:8080", "browser and API listen address")
	dbSource := settings.CampfireDatabase.String(flags, "data/panel.db", "SQLite path or PostgreSQL URL")
	dataDir := settings.CampfireData.String(flags, "", "persistent state directory; defaults beside SQLite or ./data for PostgreSQL")
	sftpListen := settings.CampfireSFTPListen.String(flags, "", "optional SFTP listen address")
	sftpKey := settings.CampfireSFTPHostKey.String(flags, "", "persistent SFTP SSH host key")
	maxUpload := settings.CampfireMaxUploadMiB.String(flags, "1024", "maximum upload size in MiB")
	publicURL := settings.CampfirePublicURL.String(flags, "", "browser origin for HTTPS reverse proxies")
	oidcIssuer := settings.CampfireOIDCIssuer.String(flags, "", "optional OIDC issuer URL")
	oidcClient := settings.CampfireOIDCClientID.String(flags, "", "OIDC client ID")
	oidcSecret := settings.CampfireOIDCClientSecret.String(flags, "", "OIDC client secret")
	agentListen := settings.CampfireAgentListen.String(flags, defaultAgentListen, "dedicated agent HTTPS listener; empty disables it")
	agentPublicURL := settings.CampfireAgentPublicURL.String(flags, "", "reachable agent HTTPS origin; defaults to the panel hostname and agent port")
	tlsCert := settings.CampfireAgentTLSCert.String(flags, "", "agent HTTPS certificate")
	tlsKey := settings.CampfireAgentTLSKey.String(flags, "", "agent HTTPS private key")
	version := flags.Bool(settings.Version.FlagName, false, "print release version and exit")
	auditRetention := settings.CampfireAuditRetentionDays.String(flags, "0", "audit record age limit in days; 0 keeps forever")
	operationRetention := settings.CampfireOperationRetentionDays.String(flags, "0", "completed operation history age limit in days; 0 keeps forever")
	gameEventRetention := settings.CampfireGameEventRetentionDays.String(flags, strconv.Itoa(int(limits.DefaultGameEventRetention/(24*time.Hour))), "game event retention in days, 1–3650")
	gameEventLimit := settings.CampfireGameEventLimit.String(flags, strconv.Itoa(limits.DefaultGameEventLimit), "retained game events per server, 1–100000")

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
		return cliutil.PrintVersion("Campfire")
	}

	eventDays, err := strconv.Atoi(*gameEventRetention)
	if err != nil || eventDays < limits.MinGameEventRetentionDays || eventDays > limits.MaxGameEventRetentionDays {
		return errors.New("game event retention must be 1–3650 days")
	}

	eventMaximum, err := strconv.Atoi(*gameEventLimit)
	if err != nil || eventMaximum < 1 || eventMaximum > limits.MaxGameEventLimit {
		return errors.New("game event limit must be 1–100000 per server")
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
		return fmt.Errorf("provide both --%s and --%s, or omit both for generated agent TLS", settings.CampfireAgentTLSCert.FlagName, settings.CampfireAgentTLSKey.FlagName)
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

	provider, err := service.NewOIDCService(ctx, auth, service.OIDCConfig{Issuer: *oidcIssuer, ClientID: *oidcClient, ClientSecret: *oidcSecret, PublicURL: *publicURL, DisableLocal: *disableLocal, AutoLinkEmail: *autoLinkEmail})
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

	games := &service.GameIntegrationService{Servers: servers}
	games.Events = service.NewGameEventService(repository.NewGameEventRepository(db.ORM()))
	games.Media = service.NewGameMediaService(repository.NewGameMediaRepository(db.ORM()))
	games.Events.Retention = time.Duration(eventDays) * 24 * time.Hour
	games.Events.MaxPerServer = eventMaximum

	if err := games.Events.Start(ctx); err != nil {
		return err
	}
	defer games.Events.Close()

	if hub != nil {
		hub.WithGameIntegrations(games.Receive)
		hub.WithGameMedia(games.Media.Receive)
	}

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

	deployment := &service.DeploymentService{Servers: servers, Sparks: sparks, Embers: embers, MaxUploadBytes: maxUploadBytes, Games: games}
	transfers := service.NewTransferService(repository.NewTransferRepository(db.ORM()), deployment)
	deployment.Transfers = transfers

	if hub != nil {
		deployment.Remote = hub
	} else if runtime != nil {
		deployment.Remote = runtime
	}

	transfers.Start(ctx)
	defer transfers.Close()

	resources := service.NewResourceHistoryService(repository.NewResourceHistoryRepository(db.ORM()), deployment)

	resources.Interval, resources.Retention, err = resourceHistoryConfig()
	if err != nil {
		return err
	}

	deployment.History = resources

	if simulator, ok := runtime.(interface {
		StartGameSimulation(context.Context, *service.GameIntegrationService) func()
	}); ok {
		defer simulator.StartGameSimulation(ctx, games)()
	}

	activity := service.NewActivityService(repository.NewActivityRepository(db.ORM()), deployment, auth)
	activity.Retention = retention

	deployment.Activity = activity
	if err := activity.Start(ctx); err != nil {
		return fmt.Errorf("start schedules: %w", err)
	}
	defer activity.Close()

	pathStyle, err := settings.CampfireS3PathStyle.BoolValue(false)
	if err != nil {
		return err
	}

	objectStore, err := backupstorage.New(ctx, backupstorage.Config{Bucket: settings.CampfireS3Bucket.Value(""), Region: settings.CampfireS3Region.Value("us-east-1"), Endpoint: settings.CampfireS3Endpoint.Value(""), Prefix: settings.CampfireS3Prefix.Value(""), PathStyle: pathStyle})
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

	resources.Notifications = notifications
	if err := resources.Start(ctx); err != nil {
		return fmt.Errorf("start resource history: %w", err)
	}
	defer resources.Close()

	imports := service.NewBackupImports(filepath.Join(stateDir, "backup-imports"), service.BackupStagingCopies*deployment.UploadLimit())
	handler := api.New(sparks, embers, servers).WithBackupImports(imports).WithAgentHub(hub).WithDeployment(deployment).WithAuthentication(auth, origin).WithOIDC(provider).WithActivity(activity).WithNotifications(notifications).Handler()
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
