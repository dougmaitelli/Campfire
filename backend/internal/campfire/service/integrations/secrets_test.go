package integrations

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestSecretsEncryptAuthenticateAndSurviveReopen(t *testing.T) {
	directory := t.TempDir()
	store := Secrets{Directory: directory}
	value := "a-private-bot-credential"

	ref, err := store.Put(value)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(directory, ref))
	if err != nil || bytes.Contains(data, []byte(value)) {
		t.Fatal("credential was stored in plaintext", err)
	}

	for _, name := range []string{"key", ref} {
		info, err := os.Stat(filepath.Join(directory, name))
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("credential or key permissions were not private", err)
		}
	}

	if read, err := (Secrets{Directory: directory}).Get(ref); err != nil || read != value {
		t.Fatal("reopened store lost the credential", err)
	}

	other, err := store.Put("another-secret")
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(directory, other), data, 0600); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Get(other); !errors.Is(err, ErrSecretUnavailable) {
		t.Fatal("ciphertext was accepted under a different reference", err)
	}

	if _, err := store.Get("../key"); !errors.Is(err, ErrSecretUnavailable) {
		t.Fatal("path-like credential reference was accepted", err)
	}

	if err := store.Delete("key"); !errors.Is(err, ErrSecretUnavailable) {
		t.Fatal("credential deletion accepted the encryption key", err)
	}

	if err := store.Delete(ref); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Get(ref); !errors.Is(err, ErrSecretUnavailable) {
		t.Fatal("deleted credential remained readable", err)
	}
}

func TestSecretsDoNotReplaceMissingOrDamagedKeys(t *testing.T) {
	store := Secrets{Directory: t.TempDir()}

	ref, err := store.Put("secret")
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(filepath.Join(store.Directory, "key")); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Put("replacement"); !errors.Is(err, ErrSecretUnavailable) {
		t.Fatal("missing key was replaced while credentials still existed", err)
	}

	if _, err := store.Get(ref); !errors.Is(err, ErrSecretUnavailable) {
		t.Fatal("missing key was not reported safely", err)
	}
}

func TestSecretsConcurrentInitialWritersShareOneKey(t *testing.T) {
	store := Secrets{Directory: t.TempDir()}
	refs := make(chan string, 8)

	var writers sync.WaitGroup
	for range 8 {
		writers.Go(func() {
			ref, err := store.Put("concurrent-secret")
			if err != nil {
				t.Error(err)
				return
			}

			refs <- ref
		})
	}

	writers.Wait()
	close(refs)

	for ref := range refs {
		if value, err := store.Get(ref); err != nil || value != "concurrent-secret" {
			t.Error("concurrent initialization changed the key", err)
		}
	}
}
