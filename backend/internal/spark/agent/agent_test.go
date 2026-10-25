package agent

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"campfire/internal/campfire/agents"
	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/agentidentity"
	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
	"campfire/internal/spark/runtime"
)

type pipeListener struct {
	incoming chan net.Conn
	done     chan struct{}
	once     sync.Once
}

type wireQuota struct{}

func (wireQuota) Prepare(string, int64) error { return nil }
func (wireQuota) Verify(string, int64) error  { return nil }

func (l *pipeListener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.incoming:
		return conn, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}
func (l *pipeListener) Close() error   { l.once.Do(func() { close(l.done) }); return nil }
func (l *pipeListener) Addr() net.Addr { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 443} }
func (l *pipeListener) dial(ctx context.Context, _ string, _ string) (net.Conn, error) {
	client, server := net.Pipe()
	select {
	case l.incoming <- server:
		return client, nil
	case <-ctx.Done():
		_ = client.Close()
		_ = server.Close()

		return nil, ctx.Err()
	case <-l.done:
		_ = client.Close()
		_ = server.Close()

		return nil, net.ErrClosed
	}
}

func waitCondition(t *testing.T, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for connection state")
		}

		time.Sleep(5 * time.Millisecond)
	}
}

func TestAgentEnrollmentConnectionAndRestart(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	db, err := database.Open(ctx, filepath.Join(dir, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	store := repository.NewEnrollmentRepository(db.ORM())
	nodes := repository.NewSparkRepository(db.ORM())

	node := model.Spark{ID: "remote-node", Name: "Remote", CreatedAt: time.Now().UTC(), SparkSettings: model.SparkSettings{PortAllocations: []model.PortAllocation{}}}
	if err := nodes.Create(ctx, node); err != nil {
		t.Fatal(err)
	}

	authority, err := agents.LoadAuthority(filepath.Join(dir, "authority.json"))
	if err != nil {
		t.Fatal(err)
	}

	reloaded, err := agents.LoadAuthority(filepath.Join(dir, "authority.json"))
	if err != nil || string(reloaded.PEM()) != string(authority.PEM()) {
		t.Fatalf("authority persistence failed: %v", err)
	}

	hub := agents.NewHub(store, authority)
	publications := make(chan protocol.GameFrame, 4)

	for _, row := range []any{
		&database.Ember{ID: "media-ember", Name: "Game", Definition: model.Ember{Schema: 1}},
		&database.Server{ID: "game-bridge", Name: "Game", SparkID: node.ID, EmberID: "media-ember", GameIntegration: true, Variables: map[string]string{}, Ports: []model.ServerPort{}, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	} {
		if err := db.ORM().Omit("Spark", "Ember", "Owner").Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}

	games := &service.GameIntegrationService{Servers: repository.NewServerRepository(db.ORM()), Events: service.NewGameEventService(repository.NewGameEventRepository(db.ORM())), Media: service.NewGameMediaService(repository.NewGameMediaRepository(db.ORM()))}
	hub.WithGameMedia(games.Media.Receive)

	hub.WithGameIntegrations(func(ctx context.Context, sparkID string, frame protocol.GameFrame) error {
		if sparkID != node.ID || frame.ServerID != "game-bridge" {
			return errors.New("incorrect publication placement")
		}

		publications <- frame

		return games.Receive(ctx, sparkID, frame)
	})
	t.Cleanup(hub.Close)

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	number, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatal(err)
	}

	template := &x509.Certificate{SerialNumber: number, Subject: pkix.Name{CommonName: "panel.test"}, DNSNames: []string{"panel.test"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}

	keyData, err := agentidentity.MarshalKey(key)
	if err != nil {
		t.Fatal(err)
	}

	cert, err := tls.X509KeyPair(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), keyData)
	if err != nil {
		t.Fatal(err)
	}

	config := hub.TLSConfig()
	config.Certificates = []tls.Certificate{cert}
	listener := &pipeListener{incoming: make(chan net.Conn), done: make(chan struct{})}
	server := &http.Server{Handler: hub.Handler(), ReadHeaderTimeout: 5 * time.Second}

	stopped := make(chan error, 1)
	go func() { stopped <- server.Serve(tls.NewListener(listener, config)) }()

	t.Cleanup(func() { hub.Close(); _ = server.Close(); _ = listener.Close(); <-stopped })

	caPath := filepath.Join(dir, "panel-ca.crt")
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
		t.Fatal(err)
	}

	token, err := hub.CreateEnrollment(ctx, node.ID)
	if err != nil {
		t.Fatal(err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)

	// Unix socket paths are bounded by the OS; use a short private fixture root.
	agentDir, err := os.MkdirTemp("", "cf-game-")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = os.RemoveAll(agentDir) })

	agentConfig := Config{PanelURL: "https://panel.test", DataDir: agentDir, CAFile: caPath, Token: token.Token, dialContext: listener.dial}
	bootstrap := agentConfig
	bootstrap.EnrollOnly = true

	bootstrap.runtimeFactory = func(context.Context, string) *runtime.Manager {
		t.Fatal("enrollment-only mode started the game runtime")
		return nil
	}
	if err := Run(ctx, bootstrap); err != nil {
		t.Fatalf("enroll-only: %v", err)
	}

	if err := agentidentity.CheckPermissions(filepath.Join(agentDir, "identity.json")); err != nil {
		t.Fatalf("enroll-only did not persist a private identity: %v", err)
	}

	if hub.Connected(node.ID) {
		t.Fatal("enrollment-only mode opened a persistent connection")
	}
	// Service startup uses only the saved identity, without the consumed token.
	agentConfig.Token = ""
	wire := &wireDocker{console: make(chan string)}
	agentConfig.runtimeFactory = func(ctx context.Context, directory string) *runtime.Manager {
		manager := runtime.New(ctx, directory)
		manager.Quota = wireQuota{}
		manager.Docker = wire
		manager.Ownership = func(string, string) error { return nil }
		manager.FileOwner = func(*os.File, string) error { return nil }

		return manager
	}

	finished := make(chan error, 1)
	go func() { finished <- Run(runCtx, agentConfig) }()

	waitCondition(t, func() bool { return hub.Connected(node.ID) })

	if hub.Info(node.ID).Hostname == "" {
		t.Fatal("agent handshake metadata missing")
	}

	info := hub.Info(node.ID)
	if info.Protocol != protocol.ProtocolVersion || len(info.Capabilities) != len(protocol.SupportedCapabilities()) {
		t.Fatal("wire handshake did not negotiate capabilities", info)
	}

	testWireCommands(t, hub, node.ID, agentDir)
	testWireGameIntegration(t, hub, node.ID, agentDir, publications)

	mediaPage, err := games.Events.Store.List(ctx, "game-bridge", repository.GameEventQuery{Limit: 10})
	if err != nil || len(mediaPage.Items) != 1 || len(mediaPage.Items[0].Attachments) != 1 {
		t.Fatal("wire media retry was not stored exactly once", mediaPage, err)
	}

	testWireRuntime(t, hub, node.ID)
	testWireTransfers(t, hub, node.ID)
	testWireConsole(t, hub, node.ID, wire)

	hub.Disconnect(node.ID)
	waitCondition(t, func() bool { return !hub.Connected(node.ID) })
	waitCondition(t, func() bool { return hub.Connected(node.ID) })

	identityPath := filepath.Join(agentDir, "identity.json")
	if err := agentidentity.CheckPermissions(identityPath); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(identityPath)
	if err != nil {
		t.Fatal(err)
	}

	var identity agentIdentity
	if err := json.Unmarshal(data, &identity); err != nil {
		t.Fatal(err)
	}

	leaf, err := identityTLS(identity)
	if err != nil || len(leaf.Certificates) != 1 {
		t.Fatalf("invalid persisted identity: %v", err)
	}

	if _, err := enrollAgent(ctx, agentConfig); err == nil {
		t.Fatal("single-use enrollment token was reused")
	}

	nodeService := service.NewSparkService(nodes, hub)

	disabled := true
	if _, err := nodeService.Update(ctx, node.ID, model.SparkChanges{Disabled: &disabled}); err != nil {
		t.Fatal(err)
	}

	if hub.Connected(node.ID) || store.Authorized(ctx, node.ID, agentidentity.Fingerprint(leaf.Certificates[0].Certificate[0])) {
		t.Fatal("disabled Spark remains connected or authorized")
	}

	cancel()

	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("agent did not stop on cancellation")
	}

	waitCondition(t, func() bool { return !hub.Connected(node.ID) })

	disabled = false
	if _, err := nodeService.Update(ctx, node.ID, model.SparkChanges{Disabled: &disabled}); err != nil {
		t.Fatal(err)
	}

	runCtx, cancel = context.WithCancel(ctx)
	t.Cleanup(cancel)

	go func() {
		finished <- Run(runCtx, Config{DataDir: agentDir, dialContext: listener.dial, runtimeFactory: agentConfig.runtimeFactory})
	}()

	waitCondition(t, func() bool { return hub.Connected(node.ID) })
	cancel()

	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("restarted agent did not stop")
	}

	waitCondition(t, func() bool { return !hub.Connected(node.ID) })
	testWireCapabilities(t, hub, node.ID, leaf, listener.dial)

	if err := db.ORM().Where("id = ?", "game-bridge").Delete(&database.Server{}).Error; err != nil {
		t.Fatal(err)
	}

	testWireRenewal(ctx, t, identityPath, node.ID, store, nodeService, listener.dial)
}

func TestAgentRejectsInsecureAndMissingConfiguration(t *testing.T) {
	for _, address := range []string{"http://panel.test", "https://user:secret@panel.test", "https://panel.test/api", "https://panel.test?token=secret"} {
		if _, err := panelTarget(address); err == nil {
			t.Errorf("accepted invalid panel URL %s", address)
		}
	}

	if err := Run(context.Background(), Config{}); err == nil {
		t.Fatal("missing data directory accepted")
	}

	if err := Run(context.Background(), Config{
		PanelURL: "https://panel.test", DataDir: t.TempDir(),
		runtimeFactory: func(context.Context, string) *runtime.Manager {
			t.Fatal("runtime initialized before enrollment validation")
			return nil
		},
	}); err == nil {
		t.Fatal("enrollment without token accepted")
	}
}

func TestEnrollmentExpiryAndRollback(t *testing.T) {
	ctx := context.Background()

	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = db.Close() }()

	nodes := repository.NewSparkRepository(db.ORM())
	if err := nodes.Create(ctx, model.Spark{ID: "node", Name: "Node", CreatedAt: time.Now().UTC(), SparkSettings: model.SparkSettings{PortAllocations: []model.PortAllocation{}}}); err != nil {
		t.Fatal(err)
	}

	store := repository.NewEnrollmentRepository(db.ORM())

	hash := agentidentity.Fingerprint([]byte("token"))
	if err := store.CreateToken(ctx, "node", hash, time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Redeem(ctx, hash, func(string) (string, error) { t.Fatal("expired token reached signer"); return "", nil }); err == nil {
		t.Fatal("expired token accepted")
	}

	if err := store.CreateToken(ctx, "node", hash, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}

	failure := errors.New("signing failure")
	if _, err := store.Redeem(ctx, hash, func(string) (string, error) { return "", failure }); !errors.Is(err, failure) {
		t.Fatal(err)
	}

	id, err := store.Redeem(ctx, hash, func(string) (string, error) { return "certificate-hash", nil })
	if err != nil || id != "node" {
		t.Fatalf("failed enrollment consumed token: %s %v", id, err)
	}

	if !store.Authorized(ctx, "node", "certificate-hash") {
		t.Fatal("redeemed identity not authorized")
	}

	if store.Authorized(ctx, "node", "other") {
		t.Fatal("unknown certificate accepted")
	}

	if _, err := store.Redeem(ctx, hash, func(string) (string, error) { return "other", nil }); err == nil {
		t.Fatal("replayed token accepted")
	}
}
