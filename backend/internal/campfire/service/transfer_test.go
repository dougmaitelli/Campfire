package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
	"campfire/internal/spark/agent"
	"campfire/internal/spark/runtime"
)

type transferQuota struct{}

func (transferQuota) Prepare(string, int64) error { return nil }
func (transferQuota) Verify(string, int64) error  { return nil }

type transferDocker struct {
	mu    sync.Mutex
	state string
}

func (d *transferDocker) Run(_ context.Context, _ io.Reader, args ...string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	switch args[0] {
	case "inspect":
		if d.state == "" {
			return "no such object", errors.New("missing")
		}

		return d.state, nil
	case "create":
		d.state = "created"
	case "start":
		d.state = "running"
	case "stop", "kill":
		d.state = "exited"
	case "rm":
		d.state = ""
	}

	return "", nil
}

type transferRouter struct {
	stores        map[string]*agent.ConfigurationStore
	mu            sync.Mutex
	loseAck       bool
	corrupt       bool
	targetOffline bool
	offsets       []int64
}

func (r *transferRouter) Deploy(ctx context.Context, id string, d model.Deployment) (model.ConfigurationReceipt, error) {
	if id == "target" && r.targetOffline {
		return model.ConfigurationReceipt{}, errors.New("destination unavailable")
	}

	return r.stores[id].Deploy(ctx, id, d)
}
func (r *transferRouter) Action(ctx context.Context, id string, q model.RuntimeRequest) (model.RuntimeStatus, error) {
	return r.stores[id].Action(ctx, id, q)
}
func (r *transferRouter) Transfer(ctx context.Context, id string, q model.TransferRequest) (model.TransferResult, error) {
	if q.Operation == "write" {
		r.mu.Lock()
		if r.corrupt {
			r.corrupt = false
			q.Data = bytes.Clone(q.Data)
			q.Data[0] ^= 1
		}
		r.mu.Unlock()
	}

	result, err := r.stores[id].Transfer(ctx, id, q)
	if q.Operation == "write" && err == nil {
		r.mu.Lock()
		defer r.mu.Unlock()

		r.offsets = append(r.offsets, q.Offset)
		if r.loseAck {
			r.loseAck = false
			return model.TransferResult{}, errors.New("simulated lost acknowledgement")
		}
	}

	return result, err
}

func TestTransferChecksumFailureRecopiesWithoutChangingPlacement(t *testing.T) {
	f := newTransferFixture(t)
	ctx := context.Background()
	f.router.corrupt = true

	item, err := f.service.Begin(ctx, f.server.ID, "target", f.ports)
	if err != nil {
		t.Fatal(err)
	}

	f.service.Start(ctx)
	t.Cleanup(f.service.Close)

	failed := waitTransfer(t, f.service, item.ID, "failed")
	if failed.ResumePhase != "copying" {
		t.Fatal("corruption did not request a new copy", failed)
	}

	server, err := f.service.Deployment.Servers.Get(ctx, f.server.ID)
	if err != nil || server.SparkID != "source" {
		t.Fatal("corrupt archive changed placement", err)
	}

	if err := f.service.Store.Retry(ctx, item.ID); err != nil {
		t.Fatal(err)
	}

	waitTransfer(t, f.service, item.ID, "complete")

	data, err := os.ReadFile(filepath.Join(f.target.Directory, f.server.ID, "data", "world.bin"))
	if err != nil || !bytes.Equal(data, f.world) {
		t.Fatal("re-copied world changed", err)
	}
}

func TestQueuedTransferCanCancelWithUnavailableDestination(t *testing.T) {
	f := newTransferFixture(t)
	ctx := context.Background()
	f.router.targetOffline = true

	item, err := f.service.Begin(ctx, f.server.ID, "target", f.ports)
	if err != nil {
		t.Fatal(err)
	}

	f.service.Start(ctx)
	t.Cleanup(f.service.Close)

	failed := waitTransfer(t, f.service, item.ID, "failed")
	if failed.ResumePhase != "queued" {
		t.Fatal(failed)
	}

	if err := f.service.Store.Cancel(ctx, item.ID); err != nil {
		t.Fatal(err)
	}

	waitTransfer(t, f.service, item.ID, "cancelled")

	if _, err := f.service.Deployment.Action(ctx, f.server.ID, "start"); err != nil {
		t.Fatal("unused offline destination prevented source recovery", err)
	}
}

type transferFixture struct {
	service        *TransferService
	db             *database.Database
	router         *transferRouter
	source, target *runtime.Manager
	server         model.Server
	ports          []model.ServerPort
	world          []byte
}

func transferManager(t *testing.T, directory string) *runtime.Manager {
	t.Helper()

	m := runtime.New(context.Background(), directory)
	m.Docker, m.Quota = &transferDocker{}, transferQuota{}
	m.Ownership = func(string, string) error { return nil }
	m.FileOwner = func(*os.File, string) error { return nil }
	t.Cleanup(m.Close)

	return m
}

func newTransferFixture(t *testing.T) *transferFixture {
	t.Helper()

	ctx := context.Background()
	directory := t.TempDir()

	db, err := database.Open(ctx, filepath.Join(directory, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	sparks := NewSparkService(repository.NewSparkRepository(db.ORM()), nil)
	for _, id := range []string{"source", "target"} {
		if err := db.ORM().Create(&database.Spark{ID: id, Name: id, MemoryMB: 128, PortAllocations: []model.PortAllocation{{BindIP: "0.0.0.0", Start: 25565, End: 25570, Protocol: "tcp"}}, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}).Error; err != nil {
			t.Fatal(err)
		}
	}

	embers := NewEmberService(repository.NewEmberRepository(db.ORM()))

	ember, err := embers.Import(ctx, model.Ember{Schema: 1, Name: "Transfer game", Images: map[string]string{"default": "alpine:3.22"}, Startup: "sleep 3600", WorkDir: "/home/container", User: "1000:1000"})
	if err != nil {
		t.Fatal(err)
	}

	for _, user := range []database.User{{ID: 1, Username: "owner", RoleID: database.UserRole, CredentialVersion: 1}, {ID: 2, Username: "operator", RoleID: database.UserRole, CredentialVersion: 1}} {
		if err := db.ORM().Omit("Role").Create(&user).Error; err != nil {
			t.Fatal(err)
		}
	}

	servers := NewServerService(repository.NewServerRepository(db.ORM()), embers)

	server, err := servers.CreateOwned(ctx, model.ServerConfiguration{Name: "World", SparkID: "source", EmberID: ember.ID, Image: "alpine:3.22", Variables: map[string]string{}, CPULimit: 1, MemoryMB: 128, DiskMB: 16, Ports: []model.ServerPort{{BindIP: "0.0.0.0", HostPort: 25565, ContainerPort: 25565, Protocol: "tcp"}}}, 1)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Omit("Server", "User", "Role").Create(&database.ServerAccessGrant{ServerID: server.ID, UserID: new(int64(2)), AccessRoleID: database.OperatorRole}).Error; err != nil {
		t.Fatal(err)
	}

	source := transferManager(t, filepath.Join(directory, "source-runtime"))
	target := transferManager(t, filepath.Join(directory, "target-runtime"))
	router := &transferRouter{stores: map[string]*agent.ConfigurationStore{
		"source": {Directory: filepath.Join(directory, "source-config"), Runtime: source},
		"target": {Directory: filepath.Join(directory, "target-config"), Runtime: target},
	}}
	deployment := &DeploymentService{Servers: servers, Sparks: sparks, Embers: embers, Remote: router}
	service := NewTransferService(repository.NewTransferRepository(db.ORM()), deployment)
	deployment.Transfers = service
	servers.WithRuntimeGuard(deployment)

	if _, err := deployment.Action(ctx, server.ID, "install"); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)

	for {
		status, err := deployment.Action(ctx, server.ID, "status")
		if err != nil {
			t.Fatal(err)
		}

		if !status.Busy {
			if status.Error != "" {
				t.Fatal(status.Error)
			}

			break
		}

		if time.Now().After(deadline) {
			t.Fatal("installation timed out")
		}

		time.Sleep(5 * time.Millisecond)
	}

	world := make([]byte, 2<<20)
	if _, err := rand.Read(world); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(source.Directory, server.ID, "data", "world.bin"), world, 0600); err != nil {
		t.Fatal(err)
	}

	backups := filepath.Join(source.Directory, server.ID, "backups")
	if err := os.MkdirAll(backups, 0700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(backups, "nightly.tar.gz"), []byte("existing completed backup"), 0600); err != nil {
		t.Fatal(err)
	}

	return &transferFixture{service: service, db: db, router: router, source: source, target: target, server: server, ports: []model.ServerPort{{BindIP: "0.0.0.0", HostPort: 25566, ContainerPort: 25565, Protocol: "tcp"}}, world: world}
}

func waitTransfer(t *testing.T, s *TransferService, id, phase string) database.ServerTransfer {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)

	for {
		item, err := s.Store.Get(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}

		if item.Phase == phase {
			return item
		}

		if item.Phase == "failed" && phase != "failed" {
			t.Fatalf("transfer failed during %s: %s", item.ResumePhase, item.Error)
		}

		if time.Now().After(deadline) {
			t.Fatalf("waiting for %s, got %+v", phase, item)
		}

		time.Sleep(10 * time.Millisecond)
	}
}

func TestTransferPreservesDataOwnershipAndReservations(t *testing.T) {
	f := newTransferFixture(t)
	ctx := context.Background()

	item, err := f.service.Begin(ctx, f.server.ID, "target", f.ports)
	if err != nil {
		t.Fatal(err)
	}

	for _, action := range []string{"start", "update", "reinstall"} {
		if _, err := f.service.Deployment.Action(ctx, f.server.ID, action); !errors.Is(err, repository.ErrTransferActive) {
			t.Fatal("transfer did not fence server actions", action, err)
		}
	}

	if _, err := f.service.Deployment.Servers.ApplyEmber(ctx, f.server.ID, f.server.EmberID, f.server.ServerConfiguration); !errors.Is(err, repository.ErrTransferActive) {
		t.Fatal("transfer did not fence Ember changes", err)
	}

	edited := f.server.ServerConfiguration

	edited.Description = "Edited during transfer"
	if saved, err := f.service.Deployment.Servers.Update(ctx, f.server.ID, edited); err != nil || saved.Description != edited.Description {
		t.Fatal("transfer blocked metadata edit", err)
	}

	edited.MemoryMB++
	if _, err := f.service.Deployment.Servers.Update(ctx, f.server.ID, edited); !errors.Is(err, repository.ErrTransferActive) {
		t.Fatal("transfer did not fence runtime edits", err)
	}

	if err := f.service.Deployment.Sparks.Delete(ctx, "source"); !errors.Is(err, repository.ErrSparkInUse) {
		t.Fatal("removed a source Spark with a server", err)
	}

	if err := f.service.Deployment.Sparks.Delete(ctx, "target"); !errors.Is(err, repository.ErrSparkInUse) {
		t.Fatal("removed a reserved destination Spark", err)
	}

	probe := f.server.ServerConfiguration

	probe.Name, probe.SparkID, probe.Ports = "Reservation probe", "target", []model.ServerPort{{BindIP: "0.0.0.0", HostPort: 25567, ContainerPort: 25565, Protocol: "tcp"}}
	if _, err := f.service.Deployment.Servers.Create(ctx, probe); !errors.Is(err, ErrServerConflict) {
		t.Fatal("destination capacity was not reserved", err)
	}

	f.service.Start(ctx)
	t.Cleanup(f.service.Close)
	waitTransfer(t, f.service, item.ID, "complete")

	server, err := f.service.Deployment.Servers.Get(ctx, f.server.ID)
	if err != nil || server.SparkID != "target" || server.Ports[0].HostPort != 25566 || server.OwnerUserID != 1 {
		t.Fatalf("placement or ownership changed incorrectly: %+v %v", server, err)
	}

	var member database.ServerAccessGrant
	if err := f.db.ORM().Where("server_id = ? AND user_id = ?", server.ID, 2).Take(&member).Error; err != nil || member.AccessRoleID != database.OperatorRole {
		t.Fatal("server membership was lost", err)
	}

	world, err := os.ReadFile(filepath.Join(f.target.Directory, server.ID, "data", "world.bin"))
	if err != nil || !bytes.Equal(world, f.world) {
		t.Fatal("world data changed", err)
	}

	backup, err := os.ReadFile(filepath.Join(f.target.Directory, server.ID, "backups", "nightly.tar.gz"))
	if err != nil || string(backup) != "existing completed backup" {
		t.Fatal("backup was not transferred", err)
	}

	if _, err := os.Stat(filepath.Join(f.source.Directory, server.ID, "data")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("source data was not cleaned", err)
	}

	if err := f.service.Store.Cancel(ctx, item.ID); !errors.Is(err, repository.ErrTransferState) {
		t.Fatal("committed transfer was cancellable", err)
	}

	if err := f.service.Deployment.Sparks.Delete(ctx, "source"); err != nil {
		t.Fatal("empty source could not be removed", err)
	}

	if _, err := f.service.Deployment.Action(ctx, server.ID, "start"); err != nil {
		t.Fatal("destination cannot start after transfer", err)
	}
}

func TestUnlimitedTransferWithoutQuotaSupport(t *testing.T) {
	f := newTransferFixture(t)
	f.source.Quota, f.target.Quota = nil, nil
	configuration := f.server.ServerConfiguration

	configuration.DiskMB = 0
	if _, err := f.service.Deployment.Servers.Update(t.Context(), f.server.ID, configuration); err != nil {
		t.Fatal(err)
	}

	item, err := f.service.Begin(t.Context(), f.server.ID, "target", f.ports)
	if err != nil {
		t.Fatal(err)
	}

	f.service.Start(t.Context())
	t.Cleanup(f.service.Close)
	waitTransfer(t, f.service, item.ID, "complete")

	data, err := os.ReadFile(filepath.Join(f.target.Directory, f.server.ID, "data", "world.bin"))
	if err != nil || !bytes.Equal(data, f.world) {
		t.Fatal("unlimited transfer did not preserve world", err)
	}

	server, err := f.service.Deployment.Servers.Get(t.Context(), f.server.ID)
	if err != nil || server.SparkID != "target" || server.DiskMB != 0 {
		t.Fatal("unlimited transfer changed allocation or placement", server, err)
	}
}

func TestTransferResumesAfterLostAcknowledgementAndSparkRestart(t *testing.T) {
	f := newTransferFixture(t)
	ctx := context.Background()
	f.router.loseAck = true

	item, err := f.service.Begin(ctx, f.server.ID, "target", f.ports)
	if err != nil {
		t.Fatal(err)
	}

	f.service.Start(ctx)
	waitTransfer(t, f.service, item.ID, "failed")
	f.service.Close()

	server, err := f.service.Deployment.Servers.Get(ctx, f.server.ID)
	if err != nil || server.SparkID != "source" {
		t.Fatal("lost acknowledgement changed placement", err)
	}

	f.target.Close()
	f.target = transferManager(t, f.target.Directory)
	f.router.stores["target"].Runtime = f.target
	restarted := NewTransferService(f.service.Store, f.service.Deployment)

	f.service.Deployment.Transfers = restarted
	if err := restarted.Store.Retry(ctx, item.ID); err != nil {
		t.Fatal(err)
	}

	restarted.Start(ctx)
	t.Cleanup(restarted.Close)
	waitTransfer(t, restarted, item.ID, "complete")
	f.router.mu.Lock()
	if len(f.router.offsets) < 2 || f.router.offsets[0] != 0 || f.router.offsets[1] != model.FileLimit {
		t.Error("copy did not resume from durable destination offset", f.router.offsets)
	}
	f.router.mu.Unlock()

	data, err := os.ReadFile(filepath.Join(f.target.Directory, f.server.ID, "data", "world.bin"))
	if err != nil || !bytes.Equal(data, f.world) {
		t.Fatal("resumed world data changed", err)
	}
}

func TestTransferCancellationPreservesSourceAndRejectsLateCommands(t *testing.T) {
	f := newTransferFixture(t)
	ctx := context.Background()
	f.router.loseAck = true

	item, err := f.service.Begin(ctx, f.server.ID, "target", f.ports)
	if err != nil {
		t.Fatal(err)
	}

	f.service.Start(ctx)
	t.Cleanup(f.service.Close)

	failed := waitTransfer(t, f.service, item.ID, "failed")
	if err := f.service.Store.Cancel(ctx, item.ID); err != nil {
		t.Fatal(err)
	}

	waitTransfer(t, f.service, item.ID, "cancelled")

	for _, request := range []model.TransferRequest{
		{ID: item.ID, ServerID: f.server.ID, Operation: "prepare", Size: failed.TotalBytes, Digest: failed.Digest},
		{ID: item.ID, ServerID: f.server.ID, Operation: "import"},
	} {
		result, err := f.router.Transfer(ctx, "target", request)
		if err == nil && result.State != "discarded" {
			t.Fatal("late destination command resurrected cancelled transfer", result)
		}
	}

	result, err := f.router.Transfer(ctx, "source", model.TransferRequest{ID: item.ID, ServerID: f.server.ID, Operation: "export"})
	if err != nil || result.State != "released" {
		t.Fatal("late export re-froze the source", result, err)
	}

	if _, err := f.router.Transfer(ctx, "source", model.TransferRequest{ID: item.ID, ServerID: f.server.ID, Operation: "cleanup"}); err == nil {
		t.Fatal("late cleanup accepted after cancellation")
	}

	data, err := os.ReadFile(filepath.Join(f.source.Directory, f.server.ID, "data", "world.bin"))
	if err != nil || !bytes.Equal(data, f.world) {
		t.Fatal("cancellation damaged original world", err)
	}

	if _, err := f.service.Deployment.Action(ctx, f.server.ID, "start"); err != nil {
		t.Fatal("source did not unlock", err)
	}
}
