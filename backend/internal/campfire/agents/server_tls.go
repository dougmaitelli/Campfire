package agents

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"sync"
	"time"

	"campfire/internal/shared/agentidentity"
)

const serverCertificateLifetime = 365 * 24 * time.Hour
const serverCertificateRenewBefore = 30 * 24 * time.Hour

// ServerTLS owns the generated HTTPS certificate and its separate private CA.
// Its CA authenticates the listener; the Spark authority authenticates clients.
type ServerTLS struct {
	mu          sync.Mutex
	path        string
	authority   *Authority
	hosts       []string
	certificate *tls.Certificate
}

// LoadServerTLS persists private TLS state beside the panel's other identities.
func LoadServerTLS(directory string, hosts []string) (*ServerTLS, error) {
	if len(hosts) == 0 {
		return nil, errors.New("agent HTTPS certificate requires a hostname or IP address")
	}

	for _, host := range hosts {
		if host == "" {
			return nil, errors.New("agent HTTPS certificate hostname is empty")
		}
	}

	authority, err := loadAuthority(filepath.Join(directory, "agent-https-authority.json"), "Campfire agent HTTPS authority")
	if err != nil {
		return nil, err
	}

	server := &ServerTLS{path: filepath.Join(directory, "agent-https.json"), authority: authority, hosts: append([]string(nil), hosts...)}

	data, err := os.ReadFile(server.path)
	if err == nil {
		if err := agentidentity.CheckPermissions(server.path); err != nil {
			return nil, err
		}

		var stored authorityFile
		if err := json.Unmarshal(data, &stored); err != nil {
			return nil, err
		}

		certificate, err := tls.X509KeyPair(stored.Certificate, stored.Key)
		if err != nil {
			return nil, err
		}

		leaf, err := x509.ParseCertificate(certificate.Certificate[0])
		if err != nil {
			return nil, err
		}

		if err := leaf.CheckSignatureFrom(authority.certificate); err != nil {
			return nil, errors.New("generated agent HTTPS certificate does not match its saved authority")
		}

		certificate.Leaf = leaf
		server.certificate = &certificate
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	if _, err := server.GetCertificate(nil); err != nil {
		return nil, err
	}

	return server, nil
}

// PEM returns only the public CA used by the installation helper.
func (s *ServerTLS) PEM() []byte { return s.authority.PEM() }

// GetCertificate renews before expiry, including without a panel restart.
func (s *ServerTLS) GetCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.certificate != nil && time.Until(s.certificate.Leaf.NotAfter) > serverCertificateRenewBefore {
		matches := true

		for _, host := range s.hosts {
			if s.certificate.Leaf.VerifyHostname(host) != nil {
				matches = false
				break
			}
		}

		if matches {
			return s.certificate, nil
		}
	}

	if err := s.authority.Refresh(); err != nil {
		return nil, err
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	number, err := certificateSerial()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	leaf := &x509.Certificate{SerialNumber: number, Subject: pkix.Name{CommonName: "Campfire agent HTTPS"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(serverCertificateLifetime), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}

	for _, host := range s.hosts {
		if ip, err := netip.ParseAddr(host); err == nil {
			leaf.IPAddresses = append(leaf.IPAddresses, ip.AsSlice())
		} else {
			leaf.DNSNames = append(leaf.DNSNames, host)
		}
	}

	s.authority.mu.RLock()
	der, err := x509.CreateCertificate(rand.Reader, leaf, s.authority.certificate, &key.PublicKey, s.authority.key)
	s.authority.mu.RUnlock()

	if err != nil {
		return nil, err
	}

	keyPEM, err := agentidentity.MarshalKey(key)
	if err != nil {
		return nil, err
	}

	certificatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})

	certificate, err := tls.X509KeyPair(certificatePEM, keyPEM)
	if err != nil {
		return nil, err
	}

	certificate.Leaf, err = x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(authorityFile{Certificate: certificatePEM, Key: keyPEM})
	if err != nil {
		return nil, err
	}

	if err := agentidentity.WritePrivate(s.path, data); err != nil {
		return nil, err
	}

	s.certificate = &certificate

	return s.certificate, nil
}
