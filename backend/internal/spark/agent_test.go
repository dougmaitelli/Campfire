package spark

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
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"campfire/internal/database"
	"campfire/internal/database/repository"
	"campfire/internal/model"
	"campfire/internal/runtime"
	"campfire/internal/service"
)

type pipeListener struct {
	incoming chan net.Conn
	done     chan struct{}
	once     sync.Once
}

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

	node := model.Spark{ID: "remote-node", Name: "Remote", Kind: model.SparkRemote, CreatedAt: time.Now().UTC(), SparkSettings: model.SparkSettings{PortAllocations: []model.PortAllocation{}}}
	if err := nodes.Create(ctx, node); err != nil {
		t.Fatal(err)
	}

	authority, err := LoadAuthority(filepath.Join(dir, "authority.json"))
	if err != nil {
		t.Fatal(err)
	}

	reloaded, err := LoadAuthority(filepath.Join(dir, "authority.json"))
	if err != nil || string(reloaded.PEM) != string(authority.PEM) {
		t.Fatalf("authority persistence failed: %v", err)
	}

	hub := NewHub(store, authority)
	t.Cleanup(hub.Close)

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	number, err := certificateSerial()
	if err != nil {
		t.Fatal(err)
	}

	template := &x509.Certificate{SerialNumber: number, Subject: pkix.Name{CommonName: "panel.test"}, DNSNames: []string{"panel.test"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}

	der, err := x509.CreateCertificate(rand.Reader, template, authority.certificate, &key.PublicKey, authority.key)
	if err != nil {
		t.Fatal(err)
	}

	keyData, err := marshalKey(key)
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
	if err := os.WriteFile(caPath, authority.PEM, 0600); err != nil {
		t.Fatal(err)
	}

	token, err := hub.CreateEnrollment(ctx, node.ID)
	if err != nil {
		t.Fatal(err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)

	agentDir := filepath.Join(dir, "agent")
	agentConfig := AgentConfig{PanelURL: "https://panel.test", DataDir: agentDir, CAFile: caPath, Token: token.Token, dialContext: listener.dial}
	wire := &wireDocker{}
	agentConfig.runtimeFactory = func(ctx context.Context, directory string) *runtime.Manager {
		manager := runtime.New(ctx, directory)
		manager.Docker = wire
		manager.Ownership = func(string, string) error { return nil }
		manager.FileOwner = func(*os.File, string) error { return nil }

		return manager
	}

	finished := make(chan error, 1)
	go func() { finished <- RunAgent(runCtx, agentConfig) }()

	waitCondition(t, func() bool { return hub.Connected(node.ID) })

	if hub.Info(node.ID).Hostname == "" {
		t.Fatal("agent handshake metadata missing")
	}

	testWireCommands(t, hub, node.ID, agentDir)
	testWireRuntime(t, hub, node.ID)

	hub.Disconnect(node.ID)
	waitCondition(t, func() bool { return !hub.Connected(node.ID) })
	waitCondition(t, func() bool { return hub.Connected(node.ID) })

	identityPath := filepath.Join(agentDir, "identity.json")
	if err := privatePermissions(identityPath); err != nil {
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

	if hub.Connected(node.ID) || store.Authorized(ctx, node.ID, fingerprint(leaf.Certificates[0].Certificate[0])) {
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

	go func() { finished <- RunAgent(runCtx, AgentConfig{DataDir: agentDir, dialContext: listener.dial}) }()

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
}

func TestAgentRejectsInsecureAndMissingConfiguration(t *testing.T) {
	for _, address := range []string{"http://panel.test", "https://user:secret@panel.test", "https://panel.test/api", "https://panel.test?token=secret"} {
		if _, err := panelTarget(address); err == nil {
			t.Errorf("accepted invalid panel URL %s", address)
		}
	}

	if err := RunAgent(context.Background(), AgentConfig{}); err == nil {
		t.Fatal("missing data directory accepted")
	}

	if err := RunAgent(context.Background(), AgentConfig{PanelURL: "https://panel.test", DataDir: t.TempDir()}); err == nil {
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
	if err := nodes.Create(ctx, model.Spark{ID: "node", Name: "Node", Kind: model.SparkRemote, CreatedAt: time.Now().UTC(), SparkSettings: model.SparkSettings{PortAllocations: []model.PortAllocation{}}}); err != nil {
		t.Fatal(err)
	}

	store := repository.NewEnrollmentRepository(db.ORM())

	hash := fingerprint([]byte("token"))
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
