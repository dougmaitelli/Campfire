package integrations

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ErrSecretUnavailable deliberately omits paths, keys, and credential contents.
var ErrSecretUnavailable = errors.New("integration credentials are unavailable; check the panel state directory")

// Secrets stores encrypted credentials separately from database configuration.
// Preserve this directory, including its key, with the panel's other private state.
type Secrets struct{ Directory string }

// Put creates a new immutable credential reference using AES-256-GCM.
func (s Secrets) Put(value string) (string, error) {
	if len(value) == 0 || len(value) > 4096 || s.Directory == "" {
		return "", ErrSecretUnavailable
	}

	if err := os.MkdirAll(s.Directory, 0700); err != nil {
		return "", ErrSecretUnavailable
	}

	key, err := s.key(true)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", ErrSecretUnavailable
	}

	gcm, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return "", ErrSecretUnavailable
	}

	ref := "secret-" + rand.Text()
	data := gcm.Seal(nil, nil, []byte(value), []byte(ref))

	file, err := os.OpenFile(filepath.Join(s.Directory, ref), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", ErrSecretUnavailable
	}

	complete := false
	defer func() {
		if !complete {
			_ = os.Remove(file.Name())
		}
	}()

	_, writeErr := file.Write(data)
	syncErr := file.Sync()

	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return "", ErrSecretUnavailable
	}

	directory, err := os.Open(s.Directory)
	if err != nil {
		return "", ErrSecretUnavailable
	}

	syncErr = directory.Sync()

	closeErr = directory.Close()
	if syncErr != nil || closeErr != nil {
		return "", ErrSecretUnavailable
	}

	complete = true

	return ref, nil
}

// Get authenticates both the encrypted contents and their reference.
func (s Secrets) Get(ref string) (string, error) {
	if !validSecretRef(ref) {
		return "", ErrSecretUnavailable
	}

	key, err := s.key(false)
	if err != nil {
		return "", err
	}

	data, err := s.read(ref, 8192)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", ErrSecretUnavailable
	}

	gcm, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return "", ErrSecretUnavailable
	}

	plain, err := gcm.Open(nil, nil, data, []byte(ref))
	if err != nil {
		return "", ErrSecretUnavailable
	}

	return string(plain), nil
}

// Delete removes a retired credential without accepting paths or key-file names.
func (s Secrets) Delete(ref string) error {
	if !validSecretRef(ref) {
		return ErrSecretUnavailable
	}

	root, err := os.OpenRoot(s.Directory)
	if err != nil {
		return ErrSecretUnavailable
	}
	defer func() { _ = root.Close() }()

	if err := root.Remove(ref); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return ErrSecretUnavailable
	}

	return nil
}

func (s Secrets) key(create bool) ([]byte, error) {
	key, err := s.read("key", 32)
	if err == nil {
		if len(key) != 32 {
			return nil, ErrSecretUnavailable
		}

		return key, nil
	}

	if !create {
		return nil, ErrSecretUnavailable
	}

	// Never silently replace a missing or damaged key for existing credentials.
	entries, err := os.ReadDir(s.Directory)
	if err != nil {
		return nil, ErrSecretUnavailable
	}

	for _, entry := range entries {
		if entry.Name() == "key" || strings.HasPrefix(entry.Name(), "secret-") {
			return s.key(false)
		}
	}

	key = make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, ErrSecretUnavailable
	}

	temporary, err := os.CreateTemp(s.Directory, ".key-")
	if err != nil {
		return nil, ErrSecretUnavailable
	}
	defer func() { _ = os.Remove(temporary.Name()) }()

	_, writeErr := temporary.Write(key)
	syncErr := temporary.Sync()

	closeErr := temporary.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return nil, ErrSecretUnavailable
	}

	// Linking publishes a complete key without replacing a concurrent creator's key.
	if err := os.Link(temporary.Name(), filepath.Join(s.Directory, "key")); err != nil {
		if !errors.Is(err, fs.ErrExist) {
			return nil, ErrSecretUnavailable
		}

		return s.key(false)
	}

	return key, nil
}

func (s Secrets) read(name string, maximum int64) ([]byte, error) {
	root, err := os.OpenRoot(s.Directory)
	if err != nil {
		return nil, ErrSecretUnavailable
	}
	defer func() { _ = root.Close() }()

	file, err := root.Open(name)
	if err != nil {
		return nil, ErrSecretUnavailable
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, ErrSecretUnavailable
	}

	data, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil || int64(len(data)) > maximum {
		return nil, ErrSecretUnavailable
	}

	return data, nil
}

func validSecretRef(ref string) bool {
	if !strings.HasPrefix(ref, "secret-") || len(ref) < 16 || len(ref) > 128 {
		return false
	}

	for _, char := range ref {
		valid := char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-'
		if !valid {
			return false
		}
	}

	return true
}
