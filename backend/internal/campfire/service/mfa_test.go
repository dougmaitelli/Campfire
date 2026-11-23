package service

import (
	"encoding/base32"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	accessperm "campfire/internal/campfire/permissions"
	"campfire/internal/campfire/service/integrations"
	"campfire/internal/shared/model"
)

func mfaFixture(t *testing.T) (*AuthService, *database.Database, string) {
	t.Helper()
	db, err := database.Open(t.Context(), filepath.Join(t.TempDir(), "mfa.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	auth := NewAuthService(repository.NewAuthRepository(db.ORM())).WithMFASecrets(integrations.Secrets{Directory: t.TempDir()})
	now := time.Unix(1900000000, 0)
	auth.now = func() time.Time { return now }
	setup, err := auth.PrepareSetup(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	_, token, err := auth.Setup(t.Context(), setup, "admin", "long test password for two factors A1!")
	if err != nil {
		t.Fatal(err)
	}
	return auth, db, token
}

func enrollMFA(t *testing.T, auth *AuthService, token string) (MFAEnrollment, []string) {
	t.Helper()
	enrollment, err := auth.BeginMFA(t.Context(), token, "Personal phone")
	if err != nil {
		t.Fatal(err)
	}
	code, err := TOTPCode(enrollment.Secret, auth.now())
	if err != nil {
		t.Fatal(err)
	}
	codes, err := auth.ConfirmMFA(t.Context(), token, code)
	if err != nil {
		t.Fatal(err)
	}
	return enrollment, codes
}

func TestTOTPRFC6238Vectors(t *testing.T) {
	// RFC 6238 appendix B SHA-1 vectors, truncated to the configured six digits.
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890"))
	for _, test := range []struct {
		at   int64
		code string
	}{{59, "287082"}, {1111111109, "081804"}, {1111111111, "050471"}, {1234567890, "005924"}, {2000000000, "279037"}, {20000000000, "353130"}} {
		code, err := TOTPCode(secret, time.Unix(test.at, 0))
		if err != nil || code != test.code {
			t.Fatal(test.at, code, err)
		}
	}
	if _, err := verifyTOTP(secret, "not-a-code", time.Unix(59, 0)); !errors.Is(err, ErrMFACode) {
		t.Fatal(err)
	}
}

func TestMFAMultipleDevicesAndIndependentRemoval(t *testing.T) {
	auth, _, token := mfaFixture(t)
	first, recovery := enrollMFA(t, auth, token)
	second, err := auth.BeginMFA(t.Context(), token, "Backup phone")
	if err != nil {
		t.Fatal(err)
	}
	code, _ := TOTPCode(second.Secret, auth.now())
	codes, err := auth.ConfirmMFA(t.Context(), token, code)
	if err != nil || len(codes) != 0 {
		t.Fatal("additional device replaced recovery codes", codes, err)
	}
	session, _, err := auth.Session(t.Context(), token)
	if err != nil {
		t.Fatal(err)
	}
	status, err := auth.MFAStatus(t.Context(), session)
	if err != nil || len(status.Devices) != 2 || status.Devices[1].Name != "Backup phone" || status.RecoveryRemaining != 10 {
		t.Fatal(status, err)
	}
	if first.Secret == second.Secret {
		t.Fatal("devices share a secret")
	}
	now := auth.now().Add(30 * time.Second)
	auth.now = func() time.Time { return now }
	for _, enrollment := range []MFAEnrollment{first, second} {
		code, _ := TOTPCode(enrollment.Secret, now)
		if _, _, err := auth.Login(t.Context(), "admin", "long test password for two factors A1!", "", code); err != nil {
			t.Fatal("device rejected", err)
		}
		if _, _, err := auth.Login(t.Context(), "admin", "long test password for two factors A1!", "", code); err == nil {
			t.Fatal("device replay accepted")
		}
	}
	// A recovery code can remove a lost device; other devices and remaining codes survive.
	if err := auth.RemoveMFADevice(t.Context(), token, status.Devices[0].ID, recovery[0]); err != nil {
		t.Fatal(err)
	}
	status, err = auth.MFAStatus(t.Context(), session)
	if err != nil || len(status.Devices) != 1 || status.Devices[0].Name != "Backup phone" || status.RecoveryRemaining != 9 {
		t.Fatal(status, err)
	}
	now = now.Add(time.Minute)
	oldCode, _ := TOTPCode(first.Secret, now)
	if _, _, err := auth.Login(t.Context(), "admin", "long test password for two factors A1!", "", oldCode); err == nil {
		t.Fatal("removed device accepted")
	}
	if err := auth.RemoveMFADevice(t.Context(), token, status.Devices[0].ID, recovery[1]); err != nil {
		t.Fatal(err)
	}
	status, err = auth.MFAStatus(t.Context(), session)
	if err != nil || status.Enabled || len(status.Devices) != 0 || status.RecoveryRemaining != 0 {
		t.Fatal(status, err)
	}
	if _, _, err := auth.Login(t.Context(), "admin", "long test password for two factors A1!", ""); err != nil {
		t.Fatal("last removal did not restore password-only login", err)
	}
}

func TestMFARequiresFactorRejectsReplayAndConsumesRecoveryOnce(t *testing.T) {
	auth, db, token := mfaFixture(t)
	enrollment, codes := enrollMFA(t, auth, token)
	var factor database.UserMFA
	if err := db.ORM().Take(&factor, "user_id = 1").Error; err != nil {
		t.Fatal(err)
	}
	if factor.SecretRef == enrollment.Secret || strings.Contains(strings.Join(factor.RecoveryHashes, ""), codes[0]) {
		t.Fatal("plaintext factor stored")
	}
	if _, _, err := auth.Login(t.Context(), "admin", "long test password for two factors A1!", ""); !errors.Is(err, repository.ErrMFARequired) {
		t.Fatal("password bypass", err)
	}
	now := auth.now().Add(time.Minute)
	auth.now = func() time.Time { return now }
	code, _ := TOTPCode(enrollment.Secret, now)
	if _, _, err := auth.Login(t.Context(), "admin", "long test password for two factors A1!", "", code); err != nil {
		t.Fatal(err)
	}
	if _, _, err := auth.Login(t.Context(), "admin", "long test password for two factors A1!", "", code); !errors.Is(err, ErrMFACode) {
		t.Fatal("replay accepted", err)
	}
	if _, _, err := auth.Login(t.Context(), "admin", "long test password for two factors A1!", "", codes[0]); err != nil {
		t.Fatal(err)
	}
	if _, _, err := auth.Login(t.Context(), "admin", "long test password for two factors A1!", "", codes[0]); !errors.Is(err, ErrMFACode) {
		t.Fatal("recovery reused", err)
	}
	if err := auth.ResetPassword(t.Context(), "replacement long password for two factors A1!"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := auth.Login(t.Context(), "admin", "replacement long password for two factors A1!", ""); !errors.Is(err, repository.ErrMFARequired) {
		t.Fatal("password reset removed MFA", err)
	}
}

func TestMFAEnrollmentBoundToSessionExpiresAndRevokesOthers(t *testing.T) {
	auth, _, token := mfaFixture(t)
	_, other, err := auth.Login(t.Context(), "admin", "long test password for two factors A1!", "")
	if err != nil {
		t.Fatal(err)
	}
	enrollment, err := auth.BeginMFA(t.Context(), token, "Personal phone")
	if err != nil {
		t.Fatal(err)
	}
	code, _ := TOTPCode(enrollment.Secret, auth.now())
	if _, err := auth.ConfirmMFA(t.Context(), other, code); !errors.Is(err, ErrMFACode) {
		t.Fatal("another session completed enrollment", err)
	}
	if _, err := auth.ConfirmMFA(t.Context(), token, code); err != nil {
		t.Fatal(err)
	}
	if _, _, err := auth.Session(t.Context(), other); !errors.Is(err, ErrAuthentication) {
		t.Fatal("other session survived enrollment", err)
	}
	if _, _, err := auth.Session(t.Context(), token); err != nil {
		t.Fatal(err)
	}
}

func TestMFAHostResetRemovesFactorAndPreservesAPITokens(t *testing.T) {
	auth, db, token := mfaFixture(t)
	sessionBefore, _, err := auth.Session(t.Context(), token)
	if err != nil {
		t.Fatal(err)
	}
	_, apiToken, err := auth.CreateAPIToken(t.Context(), PrincipalFromSession(sessionBefore), "automation", 30, []model.PermissionID{accessperm.AllAccess})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = enrollMFA(t, auth, token)
	if _, err := auth.AuthenticateAPIToken(t.Context(), apiToken); err != nil {
		t.Fatal("enrollment revoked API token", err)
	}
	if err := auth.ResetMFA(t.Context(), "admin"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := auth.Session(t.Context(), token); !errors.Is(err, ErrAuthentication) {
		t.Fatal("session survived reset", err)
	}
	_, newToken, err := auth.Login(t.Context(), "admin", "long test password for two factors A1!", "")
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := auth.Session(t.Context(), newToken)
	if err != nil {
		t.Fatal(err)
	}
	status, err := auth.MFAStatus(t.Context(), session)
	if err != nil || status.Enabled || status.RecoveryRemaining != 0 {
		t.Fatal(status, err)
	}
	if _, err := auth.AuthenticateAPIToken(t.Context(), apiToken); err != nil {
		t.Fatal("reset revoked API token", err)
	}
	var factor database.UserMFA
	if err := db.ORM().Take(&factor, "user_id = 1").Error; err != nil || factor.SecretRef != "" {
		t.Fatal(err)
	}
}

func TestMFAEnrollmentExpiryAndDisableProof(t *testing.T) {
	auth, _, token := mfaFixture(t)
	enrollment, err := auth.BeginMFA(t.Context(), token, "Personal phone")
	if err != nil {
		t.Fatal(err)
	}
	now := auth.now().Add(mfaSetupLifetime + time.Second)
	auth.now = func() time.Time { return now }
	code, _ := TOTPCode(enrollment.Secret, now)
	if _, err := auth.ConfirmMFA(t.Context(), token, code); !errors.Is(err, ErrMFACode) {
		t.Fatal("expired enrollment accepted", err)
	}
	_, codes := enrollMFA(t, auth, token)
	if err := auth.DisableMFA(t.Context(), token, "wrong", codes[0]); !errors.Is(err, ErrAuthentication) {
		t.Fatal(err)
	}
	if err := auth.DisableMFA(t.Context(), token, "long test password for two factors A1!", ""); !errors.Is(err, repository.ErrMFARequired) {
		t.Fatal("disabled without factor", err)
	}
	if err := auth.DisableMFA(t.Context(), token, "long test password for two factors A1!", codes[0]); err != nil {
		t.Fatal(err)
	}
	if _, _, err := auth.Login(t.Context(), "admin", "long test password for two factors A1!", ""); err != nil {
		t.Fatal(err)
	}
}

func TestAdminMFAResetUsesAuthenticatedSession(t *testing.T) {
	auth, _, token := mfaFixture(t)
	enrollMFA(t, auth, token)
	user, err := auth.Access().CreateUser(t.Context(), CreateUserInput{Username: "recover-me", Password: "long test password for two factors A1!", RoleID: database.UserRole})
	if err != nil {
		t.Fatal(err)
	}
	_, userToken, err := auth.Login(t.Context(), user.Username, "long test password for two factors A1!", "")
	if err != nil {
		t.Fatal(err)
	}
	enrollMFA(t, auth, userToken)
	if err := auth.AdminResetMFA(t.Context(), token, user.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := auth.Login(t.Context(), user.Username, "long test password for two factors A1!", ""); err != nil {
		t.Fatal(err)
	}
}

func TestMFARecoveryIsAtomic(t *testing.T) {
	auth, _, token := mfaFixture(t)
	_, codes := enrollMFA(t, auth, token)
	results := make(chan error, 2)
	for range 2 {
		go func() {
			_, _, err := auth.Login(t.Context(), "admin", "long test password for two factors A1!", "", codes[0])
			results <- err
		}()
	}
	successes := 0
	for range 2 {
		if <-results == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("recovery succeeded %d times", successes)
	}
	if _, _, err := auth.Session(t.Context(), token); err != nil {
		t.Fatal("recovery revoked another browser", err)
	}
}

func TestProfilePasswordChangeRequiresFactorsAndKeepsEnrollment(t *testing.T) {
	auth, _, token := mfaFixture(t)
	_, codes := enrollMFA(t, auth, token)
	password := "long test password for two factors A1!"
	replacement := "new long password for profile update A1!"
	if _, err := auth.UpdateProfile(t.Context(), token, "Name", "", replacement, "wrong", codes[0]); !errors.Is(err, ErrAuthentication) {
		t.Fatal("changed password without proof", err)
	}
	if _, err := auth.UpdateProfile(t.Context(), token, "Name", "", replacement, password, ""); !errors.Is(err, repository.ErrMFARequired) {
		t.Fatal("password change bypassed MFA", err)
	}
	_, otherToken, err := auth.Login(t.Context(), "admin", password, "", codes[1])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.UpdateProfile(t.Context(), token, "Name", "", replacement, password, codes[0]); err != nil {
		t.Fatal(err)
	}
	if _, _, err := auth.Session(t.Context(), otherToken); !errors.Is(err, ErrAuthentication) {
		t.Fatal("old session still active", err)
	}
	if _, _, err := auth.Session(t.Context(), token); err != nil {
		t.Fatal("current browser signed out", err)
	}
	if _, _, err := auth.Login(t.Context(), "admin", replacement, ""); !errors.Is(err, repository.ErrMFARequired) {
		t.Fatal("password change disabled MFA", err)
	}
}

func TestMFARecoveryCodeConcurrentUseSucceedsOnce(t *testing.T) {
	auth, _, token := mfaFixture(t)
	_, codes := enrollMFA(t, auth, token)
	results := make(chan error, 2)
	for range 2 {
		go func() {
			_, _, err := auth.Login(t.Context(), "admin", "long test password for two factors A1!", "", codes[0])
			results <- err
		}()
	}
	successes := 0
	for range 2 {
		if <-results == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("recovery code succeeded %d times", successes)
	}
}
