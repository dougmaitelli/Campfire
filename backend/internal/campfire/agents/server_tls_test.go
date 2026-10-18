package agents

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGeneratedServerTLSPersistenceRenewalAndHostnameChanges(t *testing.T) {
	directory := t.TempDir()

	server, err := LoadServerTLS(directory, []string{"agents.example.test", "127.0.0.1", "::1"})
	if err != nil {
		t.Fatal(err)
	}

	trust := server.PEM()

	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(trust) {
		t.Fatal("missing public trust certificate")
	}

	certificate, err := server.GetCertificate(nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, host := range []string{"agents.example.test", "127.0.0.1", "::1"} {
		if _, err := certificate.Leaf.Verify(x509.VerifyOptions{Roots: roots, DNSName: host}); err != nil {
			t.Fatal(host, err)
		}
	}

	if _, err := certificate.Leaf.Verify(x509.VerifyOptions{Roots: roots, DNSName: "other.example.test"}); err == nil {
		t.Fatal("accepted unconfigured hostname")
	}

	if _, err := certificate.Leaf.Verify(x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); err == nil {
		t.Fatal("server certificate can authenticate as a Spark client")
	}

	for _, name := range []string{"agent-https-authority.json", "agent-https.json"} {
		info, err := os.Stat(filepath.Join(directory, name))
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("TLS state is not private", name, err)
		}
	}

	reloaded, err := LoadServerTLS(directory, []string{"agents.example.test", "127.0.0.1", "::1"})
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(reloaded.PEM(), trust) || !bytes.Equal(reloaded.certificate.Certificate[0], certificate.Certificate[0]) {
		t.Fatal("TLS identity changed on restart")
	}

	short := *reloaded.certificate.Leaf
	short.NotAfter = time.Now().Add(time.Hour)
	reloaded.certificate.Leaf = &short

	renewed, err := reloaded.GetCertificate(nil)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(reloaded.PEM(), trust) || bytes.Equal(renewed.Certificate[0], certificate.Certificate[0]) {
		t.Fatal("renewal replaced trust or retained expiring leaf")
	}

	if _, err := renewed.Leaf.Verify(x509.VerifyOptions{Roots: roots, DNSName: "agents.example.test"}); err != nil {
		t.Fatal(err)
	}

	changed, err := LoadServerTLS(directory, []string{"new.example.test"})
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(changed.PEM(), trust) {
		t.Fatal("hostname change replaced CA")
	}

	if _, err := changed.certificate.Leaf.Verify(x509.VerifyOptions{Roots: roots, DNSName: "new.example.test"}); err != nil {
		t.Fatal(err)
	}
}

func TestGeneratedServerTLSHandshake(t *testing.T) {
	serverTLS, err := LoadServerTLS(t.TempDir(), []string{"127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}

	authority, err := LoadAuthority(filepath.Join(t.TempDir(), "spark-authority.json"))
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Equal(authority.PEM(), serverTLS.PEM()) {
		t.Fatal("client and server authorities must be separate")
	}

	hub := NewHub(nil, authority)
	server := httptest.NewUnstartedServer(hub.Handler())
	server.TLS = hub.TLSConfig()
	server.TLS.GetCertificate = serverTLS.GetCertificate

	certificate, err := serverTLS.GetCertificate(nil)
	if err != nil {
		t.Fatal(err)
	}

	server.TLS.Certificates = []tls.Certificate{*certificate}
	server.StartTLS()
	t.Cleanup(server.Close)

	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(serverTLS.PEM())
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS13}}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/spark/connect", nil)
	if err != nil {
		t.Fatal(err)
	}

	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusUnauthorized {
		t.Fatal("anonymous Spark was not rejected", response.StatusCode)
	}
}

func TestGeneratedServerTLSRejectsDamagedState(t *testing.T) {
	directory := t.TempDir()
	if _, err := LoadServerTLS(directory, []string{"localhost"}); err != nil {
		t.Fatal(err)
	}

	data, err := json.Marshal(authorityFile{Certificate: []byte("broken"), Key: []byte("broken")})
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(directory, "agent-https.json"), data, 0600); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadServerTLS(directory, []string{"localhost"}); err == nil {
		t.Fatal("silently replaced damaged TLS identity")
	}
}
