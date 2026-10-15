package agents

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"path/filepath"
	"testing"
	"time"

	"campfire/internal/shared/agentidentity"
)

func TestAuthorityRenewalPreservesExistingIdentitiesAndReloadsTLS(t *testing.T) {
	path := filepath.Join(t.TempDir(), "authority.json")

	authority, err := LoadAuthority(path)
	if err != nil {
		t.Fatal(err)
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, key)
	if err != nil {
		t.Fatal(err)
	}

	signed, err := authority.Sign("existing-spark", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csr}))
	if err != nil {
		t.Fatal(err)
	}

	block, _ := pem.Decode(signed)

	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}

	oldKey, err := agentidentity.MarshalKey(authority.key)
	if err != nil {
		t.Fatal(err)
	}

	short := *authority.certificate
	short.NotAfter = time.Now().Add(90 * 24 * time.Hour)

	der, err := x509.CreateCertificate(rand.Reader, &short, &short, &authority.key.PublicKey, authority.key)
	if err != nil {
		t.Fatal(err)
	}

	authority.certificate, err = x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}

	authority.pem = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})

	data, err := json.Marshal(authorityFile{Certificate: authority.pem, Key: oldKey})
	if err != nil {
		t.Fatal(err)
	}

	if err := agentidentity.WritePrivate(path, data); err != nil {
		t.Fatal(err)
	}
	// The TLS config already exists when the signing certificate approaches expiry.
	config := NewHub(nil, authority).TLSConfig()
	config.Certificates = []tls.Certificate{{Certificate: [][]byte{[]byte("HTTPS certificate remains configured")}}}

	current, err := config.GetConfigForClient(&tls.ClientHelloInfo{})
	if err != nil {
		t.Fatal(err)
	}

	if len(current.Certificates) != 1 || current.GetConfigForClient != nil {
		t.Fatal("renewal lost the server TLS configuration")
	}

	when := time.Now().Add(180 * 24 * time.Hour)
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: current.ClientCAs, CurrentTime: when, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); err != nil {
		t.Fatal("existing Spark identity stopped verifying after CA renewal", err)
	}

	reloaded, err := LoadAuthority(path)
	if err != nil {
		t.Fatal(err)
	}

	newKey, err := agentidentity.MarshalKey(reloaded.key)
	if err != nil || !bytes.Equal(newKey, oldKey) || time.Until(reloaded.certificate.NotAfter) < 9*365*24*time.Hour {
		t.Fatal("signing key changed or renewal was not persisted", err)
	}
}

func TestAuthorityKeepsIdentityOnSaveFailure(t *testing.T) {
	authority, err := LoadAuthority(filepath.Join(t.TempDir(), "authority.json"))
	if err != nil {
		t.Fatal(err)
	}

	before := authority.PEM()
	authority.certificate.NotAfter = time.Now().Add(time.Hour)

	authority.path = filepath.Join(authority.path, "cannot-write")
	if err := authority.Refresh(); err == nil {
		t.Fatal("CA renewal ignored persistence failure")
	}

	if !bytes.Equal(before, authority.PEM()) {
		t.Fatal("failed save replaced active authority")
	}
}
