package agent

import (
	"bytes"
	"crypto/tls"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"campfire/internal/campfire/agents"
	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
)

func TestEnrollmentAndClientRenewalWithGeneratedServerTLS(t *testing.T) {
	ctx := t.Context()
	directory := t.TempDir()

	db, err := database.Open(ctx, filepath.Join(directory, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	if err := db.ORM().Create(&database.Spark{ID: "generated-tls", Name: "Generated TLS"}).Error; err != nil {
		t.Fatal(err)
	}

	authority, err := agents.LoadAuthority(filepath.Join(directory, "spark-authority.json"))
	if err != nil {
		t.Fatal(err)
	}

	serverTLS, err := agents.LoadServerTLS(directory, []string{"agents.test"})
	if err != nil {
		t.Fatal(err)
	}

	hub := agents.NewHub(repository.NewEnrollmentRepository(db.ORM()), authority).WithEnrollmentEndpoint("https://agents.test:8443", serverTLS.PEM)
	t.Cleanup(hub.Close)
	config := hub.TLSConfig()
	config.GetCertificate = serverTLS.GetCertificate
	listener := &pipeListener{incoming: make(chan net.Conn), done: make(chan struct{})}
	server := &http.Server{Handler: hub.Handler(), ReadHeaderTimeout: 5 * time.Second}

	stopped := make(chan error, 1)
	go func() { stopped <- server.Serve(tls.NewListener(listener, config)) }()

	t.Cleanup(func() { _ = server.Close(); _ = listener.Close(); <-stopped })

	enrollment, err := hub.CreateEnrollment(ctx, "generated-tls")
	if err != nil {
		t.Fatal(err)
	}

	caFile := filepath.Join(directory, "bootstrap-ca.pem")
	if err := os.WriteFile(caFile, []byte(enrollment.ServerCA), 0600); err != nil {
		t.Fatal(err)
	}

	identity, err := enrollAgent(ctx, Config{PanelURL: enrollment.AgentURL, Token: enrollment.Token, CAFile: caFile, dialContext: listener.dial})
	if err != nil {
		t.Fatal(err)
	}

	if identity.ID != "generated-tls" || !bytes.Equal(identity.ServerCA, serverTLS.PEM()) {
		t.Fatal("enrollment did not retain CA trust")
	}

	if err := os.Remove(caFile); err != nil {
		t.Fatal(err)
	}
	// Renew using only the saved identity, after the bootstrap CA file is gone.
	// X.509 timestamps have second precision; renewal must extend the old expiry.
	time.Sleep(time.Second + 10*time.Millisecond)

	renewed, err := renewAgent(ctx, identity, filepath.Join(directory, "identity.json"), listener.dial)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(renewed.ServerCA, identity.ServerCA) {
		t.Fatal("renewal lost server CA trust")
	}
	// Replacing the public hostname cannot bypass normal hostname verification.
	identity.PanelURL = "https://unconfigured.test:8443"
	if _, err := renewAgent(ctx, identity, filepath.Join(directory, "identity.json"), listener.dial); err == nil {
		t.Fatal("accepted a server with the wrong hostname")
	}
}
