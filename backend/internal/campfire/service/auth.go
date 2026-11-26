package service

import (
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/limits"
	"campfire/internal/shared/utils"
)

// Authentication errors are safe to expose without leaking stored credentials.
var (
	ErrAuthentication = repository.ErrAuthentication
	ErrAdminExists    = repository.ErrAdminExists
	ErrAuthInput      = fmt.Errorf("use a username of %d–%d letters, numbers, dots, dashes or underscores; password requirements: %s", limits.UsernameMin, limits.UsernameMax, PasswordRequirements())
	ErrAuthRate       = errors.New("too many attempts. Try again in one minute")
)

// SessionLifetime bounds a login even while its browser continues polling.
const SessionLifetime = limits.SessionLifetime

type authAttempts struct {
	start time.Time
	count int
}

// AuthService owns initial administrator setup, account authentication and sessions.
type AuthService struct {
	store         *repository.AuthRepository
	mfaSecrets    MFASecrets
	autoLinkEmail bool
	mu            sync.Mutex
	setupHash     [32]byte
	setupReady    bool
	attempts      map[string]authAttempts
	global        authAttempts
	now           func() time.Time
	hashSlots     chan struct{}
	loginProvider string
	disableLocal  bool
}

// NewAuthService constructs authentication without enabling unprotected setup.
func NewAuthService(store *repository.AuthRepository) *AuthService {
	return &AuthService{
		store: store, attempts: make(map[string]authAttempts), now: time.Now,
		hashSlots: make(chan struct{}, 4),
	}
}

func (s *AuthService) passwordWork(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	select {
	case s.hashSlots <- struct{}{}:
		return func() { <-s.hashSlots }, nil
	default:
		return nil, ErrAuthRate
	}
}

func authToken() string {
	var bytes [32]byte

	_, _ = rand.Read(bytes[:])

	return base64.RawURLEncoding.EncodeToString(bytes[:])
}

func tokenHash(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func validAuthToken(value string) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(decoded) == 32 && len(value) == 43
}

// PrepareSetup returns a one-time console secret only while no account exists.
// Restarting before setup completes replaces the secret; only its hash is kept.
func (s *AuthService) PrepareSetup(ctx context.Context) (string, error) {
	admin, err := s.store.Credential(ctx)
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.setupHash = [32]byte{}
	s.setupReady = false

	if admin != nil {
		return "", nil
	}

	token := authToken()
	s.setupHash = sha256.Sum256([]byte(token))
	s.setupReady = true

	return token, nil
}

// NeedsSetup reads current state rather than relying on process-local setup caches.
func (s *AuthService) NeedsSetup(ctx context.Context) (bool, error) {
	admin, err := s.store.Credential(ctx)
	return admin == nil, err
}

// AllowAttempt bounds work and memory without trusting forwarded client headers.
func (s *AuthService) AllowAttempt(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	if now.Sub(s.global.start) >= limits.LoginWindow {
		s.global = authAttempts{start: now}
	}

	for address, entry := range s.attempts {
		if now.Sub(entry.start) >= limits.LoginWindow {
			delete(s.attempts, address)
		}
	}

	entry := s.attempts[key]
	if entry.start.IsZero() {
		entry.start = now
	}

	if s.global.count >= limits.LoginGlobalAttempts || entry.count >= limits.LoginClientAttempts || len(s.attempts) >= limits.LoginClients {
		return false
	}

	entry.count++
	s.global.count++
	s.attempts[key] = entry

	return true
}

// PasswordRequirements describes the configured rules for setting a password.
func PasswordRequirements() string {
	rules := []string{fmt.Sprintf("at least %d characters", limits.PasswordMin)}
	if limits.PasswordRequireUppercase {
		rules = append(rules, "an uppercase letter")
	}

	if limits.PasswordRequireLowercase {
		rules = append(rules, "a lowercase letter")
	}

	if limits.PasswordRequireNumber {
		rules = append(rules, "a number")
	}

	if limits.PasswordRequireSymbol {
		rules = append(rules, "a symbol")
	}

	return strings.Join(rules, ", ")
}

func validAccountInput(username, password string) bool {
	if !utils.IsValidUsername(username) || !utf8.ValidString(password) || utf8.RuneCountInString(password) < limits.PasswordMin || len(password) > limits.PasswordMax {
		return false
	}

	var upper, lower, number, symbol bool
	for _, char := range password {
		upper = upper || unicode.IsUpper(char)
		lower = lower || unicode.IsLower(char)
		number = number || unicode.IsDigit(char)
		symbol = symbol || unicode.IsPunct(char) || unicode.IsSymbol(char)
	}

	return (!limits.PasswordRequireUppercase || upper) &&
		(!limits.PasswordRequireLowercase || lower) &&
		(!limits.PasswordRequireNumber || number) &&
		(!limits.PasswordRequireSymbol || symbol)
}

func passwordHash(password string) (string, error) {
	salt := authToken()

	key, err := pbkdf2.Key(sha256.New, password, []byte(salt), limits.PasswordIterations, limits.PasswordKeyBytes)
	if err != nil {
		return "", err
	}

	return "$" + limits.PasswordAlgorithm + "$" + strconv.Itoa(limits.PasswordIterations) + "$" + salt + "$" + base64.RawURLEncoding.EncodeToString(key), nil
}

func verifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	valid := len(parts) == 5 && parts[1] == limits.PasswordAlgorithm && parts[2] == strconv.Itoa(limits.PasswordIterations)
	salt := "campfire-dummy-salt-for-invalid-login"

	var expected []byte

	if valid {
		salt = parts[3]
		expected, _ = base64.RawURLEncoding.DecodeString(parts[4])
	}

	key, err := pbkdf2.Key(sha256.New, password, []byte(salt), limits.PasswordIterations, limits.PasswordKeyBytes)

	return err == nil && subtle.ConstantTimeCompare(key, expected) == 1 && valid
}

func newUserSession(userID, version int64, now time.Time) (database.UserSession, string) {
	token := authToken()
	return database.UserSession{Hash: tokenHash(token), UserID: userID, CSRF: authToken(), CredentialVersion: version, CreatedAt: now.Unix(), ExpiresAt: now.Add(SessionLifetime).Unix(), LastSeenAt: now.Unix()}, token
}

// Setup consumes proof of local host access and atomically creates the administrator.
func (s *AuthService) Setup(ctx context.Context, token, username, password, email string) (database.UserSession, string, error) {
	username = strings.ToLower(strings.TrimSpace(username))

	email = utils.NormalizeEmail(email)
	if !validEmail(email) {
		return database.UserSession{}, "", repository.ErrEmailRequired
	}

	if !validAccountInput(username, password) {
		return database.UserSession{}, "", ErrAuthInput
	}

	digest := sha256.Sum256([]byte(token))

	s.mu.Lock()
	allowed := s.setupReady && validAuthToken(token) && subtle.ConstantTimeCompare(digest[:], s.setupHash[:]) == 1
	s.mu.Unlock()

	if !allowed {
		return database.UserSession{}, "", ErrAuthentication
	}

	release, err := s.passwordWork(ctx)
	if err != nil {
		return database.UserSession{}, "", err
	}
	defer release()

	encoded, err := passwordHash(password)
	if err != nil {
		return database.UserSession{}, "", err
	}

	session, raw := newUserSession(1, 1, s.now())

	session.UserAgent = sessionUserAgent(ctx)
	if err := s.store.Setup(ctx, database.User{ID: 1, Username: username, Email: email, OIDCLinkEmail: &email, PasswordHash: encoded, CredentialVersion: 1}, session); err != nil {
		return database.UserSession{}, "", err
	}

	s.mu.Lock()
	s.setupReady = false
	s.setupHash = [32]byte{}
	s.mu.Unlock()

	return session, raw, nil
}

// Login verifies credentials and rotates any current browser session.
func (s *AuthService) Login(ctx context.Context, username, password, previous string, codes ...string) (database.UserSession, string, error) {
	code := ""
	if len(codes) > 0 {
		code = codes[0]
	}

	enabled, err := s.LocalLoginEnabled(ctx)
	if err != nil {
		return database.UserSession{}, "", err
	}

	if !enabled {
		return database.UserSession{}, "", ErrAuthentication
	}

	if len(username) > limits.UsernameMax || len(password) > limits.PasswordMax {
		return database.UserSession{}, "", ErrAuthentication
	}

	release, err := s.passwordWork(ctx)
	if err != nil {
		return database.UserSession{}, "", err
	}
	defer release()

	admin, err := s.store.FindCredential(ctx, strings.ToLower(strings.TrimSpace(username)))
	if err != nil {
		return database.UserSession{}, "", err
	}

	encoded := ""
	if admin != nil {
		encoded = admin.PasswordHash
	}

	matched := verifyPassword(encoded, password)
	if !matched || admin == nil || admin.Disabled {
		return database.UserSession{}, "", ErrAuthentication
	}

	factor, err := s.store.MFA(ctx, admin.ID)
	if err != nil {
		return database.UserSession{}, "", err
	}

	if factor.SecretRef != "" && !s.AllowAttempt(fmt.Sprintf("mfa:%d", admin.ID)) {
		return database.UserSession{}, "", ErrAuthRate
	}

	proof, err := s.factorProof(factor, code)
	if err != nil {
		return database.UserSession{}, "", err
	}

	session, raw := newUserSession(admin.ID, admin.CredentialVersion, s.now())

	session.UserAgent = sessionUserAgent(ctx)
	if err := s.store.CreateSession(ctx, session, tokenHash(previous), s.now(), proof); err != nil {
		return database.UserSession{}, "", err
	}

	return session, raw, nil
}

// Session validates an opaque browser credential against persisted expiry and revocation.
func (s *AuthService) Session(ctx context.Context, token string) (database.UserSession, string, error) {
	if !validAuthToken(token) {
		return database.UserSession{}, "", ErrAuthentication
	}

	session, username, err := s.store.Session(ctx, tokenHash(token), s.now())
	if err != nil {
		return session, username, err
	}

	enabled, err := s.LocalLoginEnabled(ctx)
	if err != nil {
		return database.UserSession{}, "", err
	}

	if session.Provider == "" && !enabled || session.Provider != "" && session.Provider != s.loginProvider {
		return database.UserSession{}, "", ErrAuthentication
	}

	return session, username, err
}

// LocalLoginEnabled keeps setup and password recovery available until OIDC has an enabled administrator.
func (s *AuthService) LocalLoginEnabled(ctx context.Context) (bool, error) {
	if !s.disableLocal || s.loginProvider == "" {
		return true, nil
	}

	ready, err := s.store.HasOIDCAdmin(ctx, s.loginProvider)

	return !ready, err
}

// Logout revokes the current browser's session.
func (s *AuthService) Logout(ctx context.Context, token string) error {
	return s.store.Revoke(ctx, tokenHash(token))
}

// ResetPassword recovers the first administrator and invalidates their sessions.
func (s *AuthService) ResetPassword(ctx context.Context, password string) error {
	return s.ResetPasswordFor(ctx, "", password)
}

// ResetPasswordFor resets a selected administrator, or the first administrator by default.
func (s *AuthService) ResetPasswordFor(ctx context.Context, username, password string) error {
	username = strings.ToLower(strings.TrimSpace(username))

	if !validAccountInput("admin", password) {
		return ErrAuthInput
	}

	encoded, err := passwordHash(password)
	if err != nil {
		return fmt.Errorf("hash replacement password: %w", err)
	}

	return s.store.ResetPasswordFor(ctx, username, encoded)
}
