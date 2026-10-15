package agents

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"time"

	"campfire/internal/shared/agentidentity"
)

// Authority issues client identities for registered Sparks.
type Authority struct {
	mu          sync.RWMutex
	path        string
	certificate *x509.Certificate
	key         *ecdsa.PrivateKey
	pem         []byte
}
type authorityFile struct {
	Certificate []byte
	Key         []byte
}

// LoadAuthority creates or loads the panel's persistent private agent authority.
func LoadAuthority(path string) (*Authority, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, err
		}

		number, err := certificateSerial()
		if err != nil {
			return nil, err
		}

		now := time.Now()
		template := &x509.Certificate{SerialNumber: number, Subject: pkix.Name{CommonName: "Campfire Spark authority"}, NotBefore: now.Add(-time.Minute), NotAfter: now.AddDate(10, 0, 0), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}

		cert, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
		if err != nil {
			return nil, err
		}

		keyData, err := agentidentity.MarshalKey(key)
		if err != nil {
			return nil, err
		}

		data, err = json.Marshal(authorityFile{Certificate: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert}), Key: keyData})
		if err != nil {
			return nil, err
		}

		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return nil, err
		}

		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return nil, err
		}

		_, writeErr := file.Write(data)
		syncErr := file.Sync()

		closeErr := file.Close()
		if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}

	if err := agentidentity.CheckPermissions(path); err != nil {
		return nil, err
	}

	var stored authorityFile
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, err
	}

	block, _ := pem.Decode(stored.Certificate)
	if block == nil {
		return nil, errors.New("invalid Spark authority certificate")
	}

	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}

	key, err := parseKey(stored.Key)
	if err != nil {
		return nil, err
	}

	public, ok := certificate.PublicKey.(*ecdsa.PublicKey)
	if !ok || !public.Equal(&key.PublicKey) || !certificate.IsCA || certificate.CheckSignatureFrom(certificate) != nil || time.Now().Before(certificate.NotBefore) {
		return nil, errors.New("invalid Spark authority")
	}

	authority := &Authority{path: path, certificate: certificate, key: key, pem: stored.Certificate}
	if err := authority.Refresh(); err != nil {
		return nil, err
	}

	return authority, nil
}

// PEM returns the current public signing certificate without exposing shared memory.
func (a *Authority) PEM() []byte {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return append([]byte(nil), a.pem...)
}

func (a *Authority) valid() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	now := time.Now()

	return !now.Before(a.certificate.NotBefore) && now.Before(a.certificate.NotAfter)
}

// Refresh renews the private CA certificate before expiry while retaining its
// signing key and subject, so enrolled client identities continue to verify.
func (a *Authority) Refresh() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.refresh()
}

func (a *Authority) refresh() error {
	if time.Until(a.certificate.NotAfter) > 365*24*time.Hour {
		return nil
	}

	number, err := certificateSerial()
	if err != nil {
		return err
	}

	now := time.Now()
	template := &x509.Certificate{SerialNumber: number, Subject: a.certificate.Subject, RawSubject: a.certificate.RawSubject, SubjectKeyId: a.certificate.SubjectKeyId, NotBefore: now.Add(-time.Minute), NotAfter: now.AddDate(10, 0, 0), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &a.key.PublicKey, a.key)
	if err != nil {
		return err
	}

	key, err := agentidentity.MarshalKey(a.key)
	if err != nil {
		return err
	}

	certificate := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})

	data, err := json.Marshal(authorityFile{Certificate: certificate, Key: key})
	if err != nil {
		return err
	}

	if err := agentidentity.WritePrivate(a.path, data); err != nil {
		return err
	}

	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		return err
	}

	a.certificate, a.pem = parsed, certificate

	return nil
}

// Sign signs a validated CSR for a specific Spark; caller-supplied identities are ignored.
func (a *Authority) Sign(id string, csr []byte) ([]byte, error) {
	block, _ := pem.Decode(csr)
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		return nil, errors.New("invalid Spark CSR")
	}

	request, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return nil, err
	}

	if err := request.CheckSignature(); err != nil {
		return nil, err
	}

	public, ok := request.PublicKey.(*ecdsa.PublicKey)
	if !ok || public.Curve != elliptic.P256() {
		return nil, errors.New("spark identity must use ECDSA P-256")
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if err := a.refresh(); err != nil {
		return nil, err
	}

	number, err := certificateSerial()
	if err != nil {
		return nil, err
	}

	now := time.Now()

	template := &x509.Certificate{SerialNumber: number, Subject: pkix.Name{CommonName: id}, NotBefore: now.Add(-time.Minute), NotAfter: now.AddDate(1, 0, 0), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
	if template.NotAfter.After(a.certificate.NotAfter) {
		template.NotAfter = a.certificate.NotAfter
	}

	certificate, err := x509.CreateCertificate(rand.Reader, template, a.certificate, public, a.key)
	if err != nil {
		return nil, err
	}

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate}), nil
}

func certificateSerial() (*big.Int, error) {
	return rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
}
func parseKey(data []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("invalid Spark private key")
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	result, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("invalid Spark key type")
	}

	return result, nil
}
