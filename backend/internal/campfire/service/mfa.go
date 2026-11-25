package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" // RFC 6238 interoperable authenticator default.
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/limits"
)

const mfaSetupLifetime = 10 * time.Minute

// ErrMFACode is safe to display without exposing the secret or account state.
var ErrMFACode = errors.New("invalid or already used verification code; use a fresh authenticator code or an unused recovery code")

// MFASecrets keeps encrypted authenticator secrets outside the account database.
type MFASecrets interface {
	Put(string) (string, error)
	Get(string) (string, error)
	Delete(string) error
}

// WithMFASecrets enables enrollment using persistent encrypted private storage.
func (s *AuthService) WithMFASecrets(secrets MFASecrets) *AuthService {
	s.mfaSecrets = secrets
	return s
}

// MFAStatus contains only safe enrollment status, never credential material.
type MFAStatus struct {
	Enabled           bool            `json:"enabled"`
	Available         bool            `json:"available"`
	RecoveryRemaining int             `json:"recovery_remaining"`
	Devices           []MFADeviceInfo `json:"devices"`
}

type MFADeviceInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt int64  `json:"created_at"`
}

// MFAStatus returns status for the actual signed-in browser owner.
func (s *AuthService) MFAStatus(ctx context.Context, session database.UserSession) (MFAStatus, error) {
	factor, err := s.store.MFA(ctx, session.UserID)
	if err != nil {
		return MFAStatus{}, err
	}
	devices := make([]MFADeviceInfo, 0, len(factor.Devices))
	for _, device := range factor.Devices {
		devices = append(devices, MFADeviceInfo{ID: device.ID, Name: device.Name, CreatedAt: device.CreatedAt})
	}
	return MFAStatus{Enabled: factor.SecretRef != "", Available: session.Provider == "" && session.User.PasswordHash != "" && s.mfaSecrets != nil, RecoveryRemaining: len(factor.RecoveryHashes), Devices: devices}, nil
}

// TOTPCode returns a six-digit RFC 6238 code for a base32 secret and Unix time.
// Exported for the isolated development fixture, never used as a public endpoint.
func TOTPCode(secret string, at time.Time) (string, error) {
	return totpCounter(secret, at.Unix()/30)
}

func totpCounter(secret string, counter int64) (string, error) {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil || len(key) < 20 || counter < 0 {
		return "", ErrMFACode
	}
	var message [8]byte
	binary.BigEndian.PutUint64(message[:], uint64(counter))
	digest := hmac.New(sha1.New, key)
	_, _ = digest.Write(message[:])
	sum := digest.Sum(nil)
	offset := sum[len(sum)-1] & 15
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", value%1000000), nil
}

func verifyTOTP(secret, code string, now time.Time) (int64, error) {
	if len(code) != 6 {
		return 0, ErrMFACode
	}
	// Small clock skew tolerance; counters are persisted to prevent reuse.
	for _, delta := range []int64{0, -1, 1} {
		counter := now.Unix()/30 + delta
		expected, err := totpCounter(secret, counter)
		if err != nil {
			return 0, err
		}
		if subtle.ConstantTimeCompare([]byte(code), []byte(expected)) == 1 {
			return counter, nil
		}
	}
	return 0, ErrMFACode
}

func recoveryHash(code string) string {
	return tokenHash(strings.ToLower(strings.ReplaceAll(strings.TrimSpace(code), "-", "")))
}

func recoveryCodes() ([]string, []string) {
	codes, hashes := make([]string, 10), make([]string, 10)
	for i := range codes {
		var raw [16]byte
		_, _ = rand.Read(raw[:])
		encoded := hex.EncodeToString(raw[:])
		codes[i] = encoded[:8] + "-" + encoded[8:16] + "-" + encoded[16:24] + "-" + encoded[24:]
		hashes[i] = recoveryHash(codes[i])
	}
	return codes, hashes
}

func (s *AuthService) factorProof(factor database.UserMFA, code string) (repository.MFAProof, error) {
	proof := repository.MFAProof{SecretRef: factor.SecretRef}
	if factor.SecretRef == "" {
		return proof, nil
	}
	if code == "" {
		return proof, repository.ErrMFARequired
	}
	code = strings.TrimSpace(code)
	if len(code) > 64 {
		return proof, ErrMFACode
	}
	hash := recoveryHash(code)
	for _, saved := range factor.RecoveryHashes {
		if subtle.ConstantTimeCompare([]byte(hash), []byte(saved)) == 1 {
			proof.RecoveryHash = hash
			return proof, nil
		}
	}
	if s.mfaSecrets == nil {
		return proof, ErrMFACode
	}
	for _, device := range factor.Devices {
		secret, err := s.mfaSecrets.Get(device.SecretRef)
		if err != nil {
			continue
		}
		counter, err := verifyTOTP(secret, code, s.now())
		if err == nil && counter > device.LastCounter {
			return repository.MFAProof{SecretRef: device.SecretRef, Counter: counter}, nil
		}
	}
	return proof, ErrMFACode
}

func (s *AuthService) mfaAfterPassword(ctx context.Context, session database.UserSession, password string) (database.UserMFA, error) {
	if session.Provider != "" || !s.AllowAttempt(fmt.Sprintf("mfa:%d", session.UserID)) {
		return database.UserMFA{}, ErrAuthRate
	}
	release, err := s.passwordWork(ctx)
	if err != nil {
		return database.UserMFA{}, err
	}
	defer release()
	if len(password) > 1024 || !verifyPassword(session.User.PasswordHash, password) {
		return database.UserMFA{}, ErrAuthentication
	}

	return s.store.MFA(ctx, session.UserID)
}

// MFAEnrollment contains the new secret for an authenticated browser enrollment.
type MFAEnrollment struct {
	Secret    string `json:"secret"`
	URI       string `json:"uri"`
	ExpiresAt int64  `json:"expires_at"`
}

// BeginMFA adds a named authenticator without replacing existing devices.
func (s *AuthService) BeginMFA(ctx context.Context, token, name string) (MFAEnrollment, error) {
	if s.mfaSecrets == nil {
		return MFAEnrollment{}, repository.ErrAccessDenied
	}
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > limits.ResourceNameLength {
		return MFAEnrollment{}, repository.ErrAccessInput
	}
	session, _, err := s.Session(ctx, token)
	if err != nil {
		return MFAEnrollment{}, err
	}
	if session.Provider != "" || session.User.PasswordHash == "" {
		return MFAEnrollment{}, repository.ErrAccessDenied
	}
	if !s.AllowAttempt(fmt.Sprintf("mfa:%d", session.UserID)) {
		return MFAEnrollment{}, ErrAuthRate
	}
	var raw [20]byte
	_, _ = rand.Read(raw[:])
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw[:])
	ref, err := s.mfaSecrets.Put(secret)
	if err != nil {
		return MFAEnrollment{}, err
	}
	expires := s.now().Add(mfaSetupLifetime).Unix()
	var retired string
	err = s.store.ChangeMFA(ctx, session, s.now(), func(current *database.UserMFA) error {
		if len(current.Devices) >= limits.MFADevicesMax {
			return repository.ErrAccessInput
		}
		retired = current.PendingRef
		current.PendingRef, current.PendingSession, current.PendingExpires = ref, session.Hash, expires
		current.PendingName = name
		return nil
	}, nil, false)
	if err != nil {
		_ = s.mfaSecrets.Delete(ref)
		return MFAEnrollment{}, err
	}
	if retired != "" {
		_ = s.mfaSecrets.Delete(retired)
	}
	uri := "otpauth://totp/" + url.PathEscape("Campfire:"+session.User.Username) + "?" + url.Values{"secret": {secret}, "issuer": {"Campfire"}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}.Encode()
	return MFAEnrollment{Secret: secret, URI: uri, ExpiresAt: expires}, nil
}

// ConfirmMFA activates a session-bound pending enrollment and returns recovery codes once.
func (s *AuthService) ConfirmMFA(ctx context.Context, token, code string) ([]string, error) {
	session, _, err := s.Session(ctx, token)
	if err != nil {
		return nil, err
	}
	if session.Provider != "" || s.mfaSecrets == nil {
		return nil, ErrAuthentication
	}
	if !s.AllowAttempt(fmt.Sprintf("mfa:%d", session.UserID)) {
		return nil, ErrAuthRate
	}
	factor, err := s.store.MFA(ctx, session.UserID)
	if err != nil {
		return nil, err
	}
	if factor.PendingRef == "" || factor.PendingSession != session.Hash || factor.PendingExpires <= s.now().Unix() {
		return nil, ErrMFACode
	}
	secret, err := s.mfaSecrets.Get(factor.PendingRef)
	if err != nil {
		return nil, err
	}
	counter, err := verifyTOTP(secret, strings.TrimSpace(code), s.now())
	if err != nil {
		return nil, err
	}
	codes := []string{}
	err = s.store.ChangeMFA(ctx, session, s.now(), func(current *database.UserMFA) error {
		if current.PendingRef != factor.PendingRef || current.PendingSession != session.Hash || current.PendingExpires <= s.now().Unix() {
			return ErrMFACode
		}
		if len(current.Devices) >= limits.MFADevicesMax {
			return repository.ErrAccessInput
		}
		if len(current.Devices) == 0 {
			codes, current.RecoveryHashes = recoveryCodes()
			current.SecretRef, current.LastCounter = current.PendingRef, counter
		}
		name := current.PendingName
		if name == "" {
			name = "Authenticator"
		}
		current.Devices = append(current.Devices, database.MFADevice{ID: authToken(), Name: name, SecretRef: current.PendingRef, LastCounter: counter, CreatedAt: s.now().Unix()})
		current.PendingRef, current.PendingSession, current.PendingExpires = "", "", 0
		current.PendingName = ""
		return nil
	}, nil, true)
	if err != nil {
		return nil, err
	}
	return codes, nil
}

// RemoveMFADevice verifies a fresh factor from any enrolled device or recovery code.
func (s *AuthService) RemoveMFADevice(ctx context.Context, token, id, code string) error {
	session, _, err := s.Session(ctx, token)
	if err != nil {
		return err
	}
	if session.Provider != "" {
		return repository.ErrAccessDenied
	}
	if !s.AllowAttempt(fmt.Sprintf("mfa:%d", session.UserID)) {
		return ErrAuthRate
	}
	factor, err := s.store.MFA(ctx, session.UserID)
	if err != nil {
		return err
	}
	proof, err := s.factorProof(factor, code)
	if err != nil {
		return err
	}
	var retired []string
	err = s.store.ChangeMFA(ctx, session, s.now(), func(current *database.UserMFA) error {
		index := slices.IndexFunc(current.Devices, func(device database.MFADevice) bool { return device.ID == id })
		if index < 0 {
			return repository.ErrAccessInput
		}
		retired = append(retired, current.Devices[index].SecretRef)
		current.Devices = slices.Delete(current.Devices, index, index+1)
		if len(current.Devices) == 0 {
			retired = append(retired, current.PendingRef)
			*current = database.UserMFA{UserID: session.UserID, RecoveryHashes: []string{}, Devices: []database.MFADevice{}}
		} else {
			current.SecretRef, current.LastCounter = current.Devices[0].SecretRef, current.Devices[0].LastCounter
		}
		return nil
	}, &proof, true)
	if err != nil {
		return err
	}
	for _, ref := range retired {
		if ref != "" && s.mfaSecrets != nil {
			_ = s.mfaSecrets.Delete(ref)
		}
	}
	return nil
}

// DisableMFA requires password and a fresh factor proof, and signs out other browsers.
func (s *AuthService) DisableMFA(ctx context.Context, token, password, code string) error {
	session, _, err := s.Session(ctx, token)
	if err != nil {
		return err
	}

	factor, err := s.mfaAfterPassword(ctx, session, password)
	if err != nil {
		return err
	}
	proof, err := s.factorProof(factor, code)
	if err != nil {
		return err
	}
	err = s.store.ChangeMFA(ctx, session, s.now(), func(current *database.UserMFA) error {
		*current = database.UserMFA{UserID: session.UserID, RecoveryHashes: []string{}}
		return nil
	}, &proof, true)
	if err != nil {
		return err
	}
	refs := []string{factor.PendingRef}
	for _, device := range factor.Devices {
		refs = append(refs, device.SecretRef)
	}
	for _, ref := range refs {
		if ref != "" {
			_ = s.mfaSecrets.Delete(ref)
		}
	}
	return nil
}

// ResetMFA requires local host access; HTTP handlers must not expose it.
func (s *AuthService) ResetMFA(ctx context.Context, username string) error {
	if strings.TrimSpace(username) == "" {
		return repository.ErrAccessInput
	}
	return s.store.ResetMFA(ctx, strings.TrimSpace(username))
}

// AdminResetMFA allows an authenticated administrator browser to recover another account.
func (s *AuthService) AdminResetMFA(ctx context.Context, token string, userID int64) error {
	session, _, err := s.Session(ctx, token)
	if err != nil {
		return err
	}
	if session.User.RoleID != database.AdminRole || session.UserID == userID {
		return repository.ErrAccessDenied
	}
	return s.store.AdminResetMFA(ctx, session, userID, s.now())
}
