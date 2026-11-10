//go:build e2e && !production

// Standalone browser-test harness. The real API and repositories use disposable
// databases; only the Spark transport is simulated. Never compiled into releases.
package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"campfire/internal/campfire/api"
	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/devfixture"
	integrationfixture "campfire/internal/campfire/devfixture/integrations"
	"campfire/internal/campfire/service"
	integrationservice "campfire/internal/campfire/service/integrations"
	"campfire/internal/campfire/service/integrations/discord"
	"campfire/internal/shared/model"
)

var fixed = time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)

const origin = "http://127.0.0.1:8099"

func must(err error) {
	if err != nil {
		panic(err)
	}
}

// No Docker, agent sockets, credentials, or external network clients are used.
type runtime struct {
	mu              sync.Mutex
	statuses        map[string]model.RuntimeStatus
	deployments     map[string]model.Deployment
	activeRevisions map[string]string
	files           map[string]map[string]string
	fileModes       map[string]map[string]uint32
	backups         map[string][]model.FileEntry
	archives        map[string][]byte
	uploads         map[string]*uploadFixture
	jobs            map[string]*jobFixture
	updates         chan struct{}
}

type uploadFixture struct {
	serverID string
	path     string
	size     int64
	data     []byte
}

func (*runtime) Connected(id string) bool { return id != "mock-offline" }

func (r *runtime) Deploy(_ context.Context, _ string, input model.Deployment) (model.ConfigurationReceipt, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.deployments == nil {
		r.deployments = make(map[string]model.Deployment)
	}
	if r.activeRevisions == nil {
		r.activeRevisions = make(map[string]string)
	}
	status := r.statuses[input.ServerID]
	if status.State == model.RuntimeStateRunning && r.activeRevisions[input.ServerID] == "" {
		r.activeRevisions[input.ServerID] = model.ContainerRevision(input)
	}
	r.deployments[input.ServerID] = input
	status.RestartRequired = status.State == model.RuntimeStateRunning && r.activeRevisions[input.ServerID] != model.ContainerRevision(input)
	r.statuses[input.ServerID] = status
	if r.files[input.ServerID] == nil {
		r.files[input.ServerID] = devfixture.Files()
	}
	return model.ConfigurationReceipt{ServerID: input.ServerID, Revision: model.DeploymentHash(input)}, nil
}

func (r *runtime) Action(_ context.Context, _ string, input model.RuntimeRequest) (model.RuntimeStatus, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	status := r.statuses[input.ServerID]
	if status.State == "" {
		status.State = "not_installed"
	}
	status.ServerID = input.ServerID
	if input.Action != "status" {
		status.OperationID, status.Operation = input.OperationID, input.Action
		status.OperationState = model.OperationSucceeded
		switch input.Action {
		case "start", "restart":
			if input.Action == model.ServerActionRestart || status.State != model.RuntimeStateRunning {
				r.activeRevisions[input.ServerID] = model.ContainerRevision(r.deployments[input.ServerID])
				status.RestartRequired = false
			}
			status.State = "running"
		case "stop", "kill":
			status.State = "stopped"
		case "delete":
			status.State = model.RuntimeStateDeleted
			delete(r.files, input.ServerID)
		case "command":
			status.Logs += input.Content + "\nCommand accepted.\n"
		case "restore":
			if err := r.restoreBackup(input); err != nil {
				return status, err
			}
			status.State = model.RuntimeStateStopped
		case "backup":
			archiveID := input.OperationID
			if input.ArchiveID != "" {
				archiveID = input.ArchiveID
			}
			name := input.BackupOptions.Mode.ArchiveName(archiveID)
			data, err := r.createBackup(input)
			if err != nil {
				return status, err
			}

			r.archives[input.ServerID+"/"+name] = data
			r.backups[input.ServerID] = append(r.backups[input.ServerID], model.FileEntry{Portable: input.BackupOptions.Definition != nil, BackupMode: input.BackupOptions.Mode.Normalized(), Name: name, Kind: model.FileKindFile, Size: int64(len(data)), ModifiedAt: fixed})
		}
		r.matchLogAlerts(input, &status)
		r.statuses[input.ServerID] = status
		close(r.updates)
		r.updates = make(chan struct{})
	}
	return status, nil
}

func seed(ctx context.Context, db *database.Database, auth *service.AuthService) *runtime {
	token, err := auth.PrepareSetup(ctx)
	must(err)
	_, _, err = auth.Setup(ctx, token, "admin", "campfire-e2e-password")
	must(err)
	fixtures, err := devfixture.Seed(ctx, db.ORM(), fixed)
	must(err)
	r := &runtime{statuses: make(map[string]model.RuntimeStatus), files: make(map[string]map[string]string), backups: make(map[string][]model.FileEntry), archives: make(map[string][]byte), updates: make(chan struct{})}
	nodeUsage := make(map[string]model.ResourceUsage)
	for i, fixture := range fixtures {
		id := fixture.ID
		r.statuses[id] = model.RuntimeStatus{ServerID: id, State: fixture.State, Ready: true, Logs: devfixture.ConsoleLogs}
		cpu, memory, disk := float64(18+i*7), int64(1536+i*512)*1024*1024, int64(4096+i*1024)*1024*1024
		if fixture.State != "running" {
			cpu, memory = 0, 0
		}
		now := time.Now().Unix()
		usage := model.ResourceUsage{ServerID: id, State: fixture.State, CPUPercent: &cpu, MemoryBytes: &memory, DiskBytes: &disk, SampledAt: now, DiskSampledAt: now}
		must(db.ORM().Omit("Server").Create(&database.ServerResourceSample{ServerID: id, SparkID: fixture.SparkID, CollectedAt: now, ReceivedAt: now, CPULimit: fixture.CPULimit, MemoryMB: fixture.MemoryMB, DiskMB: fixture.DiskMB, Usage: usage}).Error)
		total, ok := nodeUsage[fixture.SparkID]
		if !ok {
			total = model.ResourceUsage{State: "connected", CPUPercent: new(float64), MemoryBytes: new(int64), DiskBytes: new(int64), SampledAt: now, DiskSampledAt: now}
		}

		*total.CPUPercent += cpu
		*total.MemoryBytes += memory
		*total.DiskBytes += disk
		nodeUsage[fixture.SparkID] = total
		r.files[id] = devfixture.Files()
		for _, entry := range devfixture.Backups(fixed) {
			data, err := devfixture.Archive(devfixture.BackupFiles(r.files[id], id, entry.Name), entry.ModifiedAt)
			must(err)
			entry.Size = int64(len(data))
			r.backups[id] = append(r.backups[id], entry)
			r.archives[id+"/"+entry.Name] = data
			must(db.ORM().Create(&database.Backup{ServerID: id, Name: entry.Name, Size: entry.Size, CreatedAt: entry.ModifiedAt.UnixNano(), Local: true, Key: id + "/" + entry.Name, NoRemote: true}).Error)
		}
	}
	var nodes []database.Spark
	must(db.ORM().Find(&nodes).Error)
	for _, node := range nodes {
		now := time.Now().Unix()
		usage, ok := nodeUsage[node.ID]
		if !ok {
			usage = model.ResourceUsage{State: "connected", CPUPercent: new(float64), MemoryBytes: new(int64), DiskBytes: new(int64), SampledAt: now, DiskSampledAt: now}
		}

		must(db.ORM().Omit("Spark").Create(&database.SparkResourceSample{SparkID: node.ID, CollectedAt: now, ReceivedAt: now, CPULimit: node.CPULimit, MemoryMB: node.MemoryMB, DiskMB: node.DiskMB, Usage: usage}).Error)
	}

	return r
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	root, err := os.MkdirTemp("", "campfire-e2e-")
	must(err)
	defer func() { must(os.RemoveAll(root)) }()
	discordFixture := integrationfixture.NewDiscord()
	discordAPI := httptest.NewServer(discordFixture.Handler())
	defer discordAPI.Close()
	discordViewer := discordFixture.ViewerHandler()
	var mu sync.RWMutex
	var handler http.Handler
	var deliveryFixture http.Handler
	var db *database.Database
	var statuses *service.RuntimeStatusService
	var copies *service.CopyService
	var integration *integrationservice.DiscordIntegrationService
	generation := 0
	reset := func() {
		if integration != nil {
			integration.Close()
		}
		discordFixture.Reset()
		if copies != nil {
			copies.Close()
		}
		if statuses != nil {
			statuses.Close()
		}
		if db != nil {
			must(db.Close())
		}
		generation++
		db, err = database.Open(ctx, filepath.Join(root, fmt.Sprintf("%d.db", generation)))
		must(err)
		auth := service.NewAuthService(repository.NewAuthRepository(db.ORM()))
		remote := seed(ctx, db, auth)
		embers := service.NewEmberService(repository.NewEmberRepository(db.ORM()))
		embers.Updates = service.NewEmberUpdateService(embers, repository.NewEmberUpdateRepository(db.ORM()))
		servers := service.NewServerService(repository.NewServerRepository(db.ORM()), embers)
		sparks := service.NewSparkService(repository.NewSparkRepository(db.ORM()), remote)
		sparks.Updates = service.NewSparkUpdateService(sparks, nil)
		deployment := &service.DeploymentService{Servers: servers, Sparks: sparks, Embers: embers, Remote: remote}
		deployment.Games = &service.GameIntegrationService{Servers: servers, Events: service.NewGameEventService(repository.NewGameEventRepository(db.ORM()))}
		deployment.Games.Media = service.NewGameMediaService(repository.NewGameMediaRepository(db.ORM()))
		deployment.History = service.NewResourceHistoryService(repository.NewResourceHistoryRepository(db.ORM()), deployment)
		deployment.Backups = service.NewBackupService(repository.NewBackupRepository(db.ORM()), deployment, nil)
		deployment.Transfers = service.NewTransferService(repository.NewTransferRepository(db.ORM()), deployment)
		copies = service.NewCopyService(repository.NewCopyRepository(db.ORM()), deployment)
		deployment.Copies = copies
		copies.Start(ctx)
		activity := service.NewActivityService(repository.NewActivityRepository(db.ORM()), deployment, auth)
		deployment.Activity = activity
		notifications := service.NewNotificationService(repository.NewNotificationRepository(db.ORM()), deployment, auth)
		deployment.LogAlerts = &service.LogAlertService{Store: repository.NewLogAlertRepository(db.ORM()), Notifications: notifications}
		statuses = service.NewRuntimeStatusService(deployment)
		must(statuses.Collect(ctx))
		statuses.Start(ctx)
		integration = integrationservice.NewDiscordIntegrationService(db.ORM(), integrationservice.Secrets{Directory: filepath.Join(root, fmt.Sprintf("secrets-%d", generation))}, discord.NewClient(discordAPI.URL, discordAPI.Client())).WithCards(deployment)
		integration.Start(ctx)
		deliveryFixture = integrationfixture.DeliveryHandler(db.ORM(), fixed)
		handler = api.New(sparks, embers, servers).WithIntegrations(integration).WithDeployment(deployment).WithAuthentication(auth, origin).WithActivity(activity).WithNotifications(notifications).WithClock(func() time.Time { return fixed }).Handler()
	}
	reset()
	defer func() { integration.Close(); copies.Close(); statuses.Close(); must(db.Close()) }()
	files := http.FileServer(http.Dir("web/dist"))
	mux := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/__test/reset" {
			if req.Method != http.MethodPost || req.Header.Get("X-Campfire-E2E") != "reset" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			// Preserve the real login cookie while removing login timestamps and
			// audit noise before capturing deterministic documentation states.
			var sessions []database.UserSession
			if req.URL.Query().Get("keep_session") == "true" {
				must(db.ORM().Find(&sessions).Error)
			}
			reset()
			for _, session := range sessions {
				must(db.ORM().Omit("User").Create(&session).Error)
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		mu.RLock()
		defer mu.RUnlock()
		if req.URL.Path == "/mock/discord" || strings.HasPrefix(req.URL.Path, "/mock/discord/") {
			discordViewer.ServeHTTP(w, req)
			return
		}
		if strings.HasPrefix(req.URL.Path, "/__test/integrations/") {
			deliveryFixture.ServeHTTP(w, req)
			return
		}
		if strings.HasPrefix(req.URL.Path, "/api/") {
			handler.ServeHTTP(w, req)
			return
		}
		files.ServeHTTP(w, req)
	})
	listener, err := net.Listen("tcp", "127.0.0.1:8099")
	must(err)
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_ = server.Shutdown(shutdown)
	}()
	fmt.Println("Campfire E2E listening on 127.0.0.1:8099")
	if err := server.Serve(listener); err != http.ErrServerClosed {
		must(err)
	}
}
