// Package agentidentity provides shared agent identity encoding and private storage.
package agentidentity

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
)

// MarshalKey encodes an agent or authority private key as PKCS#8 PEM.
func MarshalKey(key *ecdsa.PrivateKey) ([]byte, error) {
	data, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}

	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: data}), nil
}

// Fingerprint returns a SHA-256 digest for certificates, tokens and definitions.
func Fingerprint(data []byte) string { hash := sha256.Sum256(data); return hex.EncodeToString(hash[:]) }

// CheckPermissions requires a private regular file.
func CheckPermissions(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("%s must be a private file (0600)", path)
	}

	return nil
}

// WritePrivate atomically writes a private file and synchronizes its contents.
func WritePrivate(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	file, err := os.CreateTemp(filepath.Dir(path), ".identity-*")
	if err != nil {
		return err
	}

	defer func() { _ = os.Remove(file.Name()) }()

	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}

	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}

	if err := file.Close(); err != nil {
		return err
	}

	if err := os.Rename(file.Name(), path); err != nil {
		return err
	}

	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}

	defer func() { _ = directory.Close() }()

	return directory.Sync()
}
