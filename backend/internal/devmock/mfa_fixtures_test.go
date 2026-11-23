//go:build !production

package devmock

import (
	"path/filepath"
	"testing"

	"campfire/internal/campfire/service/integrations"
)

func TestMFAExamplesSupportRecoveryAndPreserveReset(t *testing.T) {
	runtime, _, auth, _ := fixture(t)
	directory := t.TempDir()
	auth.WithMFASecrets(integrations.Secrets{Directory: filepath.Join(directory, "auth-secrets")})
	if err := runtime.EnableMFAExamples(t.Context(), auth, directory); err != nil {
		t.Fatal(err)
	}
	if _, _, err := auth.Login(t.Context(), "mfa-demo", Password, ""); err == nil {
		t.Fatal("example did not require 2FA")
	}
	if _, _, err := auth.Login(t.Context(), "mfa-demo", Password, "", MockMFARecoveryCode); err != nil {
		t.Fatal(err)
	}
	if _, _, err := auth.Login(t.Context(), "mfa-demo", Password, "", MockMFARecoveryCode); err == nil {
		t.Fatal("example recovery code reused")
	}
	if err := auth.ResetMFA(t.Context(), "mfa-demo"); err != nil {
		t.Fatal(err)
	}
	if err := runtime.EnableMFAExamples(t.Context(), auth, directory); err != nil {
		t.Fatal(err)
	}
	if _, _, err := auth.Login(t.Context(), "mfa-demo", Password, ""); err != nil {
		t.Fatal("mock restart replaced the reset", err)
	}
}
