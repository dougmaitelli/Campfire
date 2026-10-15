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
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"campfire/internal/database"
	"campfire/internal/database/repository"
)

// Authentication errors are safe to expose without leaking stored credentials.
var (
	ErrAuthentication = repository.ErrAuthentication
	ErrAdminExists    = repository.ErrAdminExists
	ErrAuthInput      = errors.New("use a username of 3–64 letters, numbers, dots, dashes or underscores and a password of at least 15 characters (maximum 1024 bytes)")
	ErrAuthRate       = errors.New("too many sign-in attempts; try again in one minute")
)

// SessionLifetime bounds a login even while its browser continues polling.
const SessionLifetime = 12 * time.Hour

var adminName = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{2,63}$`)

type authAttempts struct {
	start time.Time
	count int
}

// AuthService owns single-admin setup, password verification and session secrets.
type AuthService struct {
	store      *repository.AuthRepository
	mu         sync.Mutex
	setupHash  [32]byte
	setupPath  string
	setupReady bool
	attempts   map[string]authAttempts
	global     authAttempts
	now        func() time.Time
	hashSlots  chan struct{}
}

// NewAuthService constructs authentication without enabling unprotected setup.
func NewAuthService(store *repository.AuthRepository) *AuthService {
	return &AuthService{store: store, attempts: make(map[string]authAttempts), now: time.Now, hashSlots: make(chan struct{}, 4)}
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

// PrepareSetup creates or loads a private bootstrap token only while no admin exists.
func (s *AuthService) PrepareSetup(ctx context.Context, path string) error {
	admin, err := s.store.Credential(ctx)
	if err != nil {
		return err
	}

	if admin != nil {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return errors.New("could not remove consumed setup token file")
		}

		return nil
	}

	token := authToken()

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if err == nil {
		_, writeErr := io.WriteString(file, token+"\n")
		syncErr := file.Sync()

		closeErr := file.Close()
		if writeErr != nil || syncErr != nil || closeErr != nil {
			_ = os.Remove(path)
			return errors.New("could not persist setup token")
		}
	} else if errors.Is(err, os.ErrExist) {
		file, err = os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if err != nil {
			return errors.New("could not read private setup token")
		}
		defer func() { _ = file.Close() }()

		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
			return errors.New("setup token must be a private regular file (mode 0600)")
		}

		data, err := io.ReadAll(io.LimitReader(file, 128))
		if err != nil {
			return errors.New("could not read setup token")
		}

		token = strings.TrimSpace(string(data))
		if !validAuthToken(token) {
			return errors.New("setup token file is invalid")
		}
	} else {
		return errors.New("could not create private setup token file")
	}

	s.mu.Lock()
	s.setupHash = sha256.Sum256([]byte(token))
	s.setupPath = path
	s.setupReady = true
	s.mu.Unlock()

	return nil
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
	if now.Sub(s.global.start) >= time.Minute {
		s.global = authAttempts{start: now}
	}

	for address, entry := range s.attempts {
		if now.Sub(entry.start) >= time.Minute {
			delete(s.attempts, address)
		}
	}

	entry := s.attempts[key]
	if entry.start.IsZero() {
		entry.start = now
	}

	if s.global.count >= 100 || entry.count >= 10 || len(s.attempts) >= 4096 {
		return false
	}

	entry.count++
	s.global.count++
	s.attempts[key] = entry

	return true
}

func validAdminInput(username, password string) bool {
	return adminName.MatchString(username) && utf8.ValidString(password) && utf8.RuneCountInString(password) >= 15 && len(password) <= 1024
}

func passwordHash(password string) (string, error) {
	salt := authToken()

	key, err := pbkdf2.Key(sha256.New, password, []byte(salt), 600000, 32)
	if err != nil {
		return "", err
	}

	return "$pbkdf2-sha256$600000$" + salt + "$" + base64.RawURLEncoding.EncodeToString(key), nil
}

func verifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	valid := len(parts) == 5 && parts[1] == "pbkdf2-sha256" && parts[2] == "600000"
	salt := "campfire-dummy-salt-for-invalid-login"

	var expected []byte

	if valid {
		salt = parts[3]
		expected, _ = base64.RawURLEncoding.DecodeString(parts[4])
	}

	key, err := pbkdf2.Key(sha256.New, password, []byte(salt), 600000, 32)

	return err == nil && subtle.ConstantTimeCompare(key, expected) == 1 && valid
}

func newAdminSession(version int64, now time.Time) (database.AdminSession, string) {
	token := authToken()
	return database.AdminSession{Hash: tokenHash(token), CSRF: authToken(), CredentialVersion: version, ExpiresAt: now.Add(SessionLifetime).Unix(), LastSeenAt: now.Unix()}, token
}

// Setup consumes proof of local host access and atomically creates the administrator.
func (s *AuthService) Setup(ctx context.Context, token, username, password string) (database.AdminSession, string, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if !validAdminInput(username, password) {
		return database.AdminSession{}, "", ErrAuthInput
	}

	digest := sha256.Sum256([]byte(token))

	s.mu.Lock()
	allowed := s.setupReady && validAuthToken(token) && subtle.ConstantTimeCompare(digest[:], s.setupHash[:]) == 1
	s.mu.Unlock()

	if !allowed {
		return database.AdminSession{}, "", ErrAuthentication
	}

	release, err := s.passwordWork(ctx)
	if err != nil {
		return database.AdminSession{}, "", err
	}
	defer release()

	encoded, err := passwordHash(password)
	if err != nil {
		return database.AdminSession{}, "", err
	}

	session, raw := newAdminSession(1, s.now())
	if err := s.store.Setup(ctx, database.AdminCredential{ID: 1, Username: username, PasswordHash: encoded, Version: 1}, session); err != nil {
		return database.AdminSession{}, "", err
	}

	s.mu.Lock()
	s.setupReady = false
	path := s.setupPath
	s.mu.Unlock()

	// The database is authoritative: any leftover token cannot configure a second admin.
	_ = os.Remove(path)

	return session, raw, nil
}

// Login verifies credentials and rotates any current browser session.
func (s *AuthService) Login(ctx context.Context, username, password, previous string) (database.AdminSession, string, error) {
	if len(username) > 64 || len(password) > 1024 {
		return database.AdminSession{}, "", ErrAuthentication
	}

	release, err := s.passwordWork(ctx)
	if err != nil {
		return database.AdminSession{}, "", err
	}
	defer release()

	admin, err := s.store.Credential(ctx)
	if err != nil {
		return database.AdminSession{}, "", err
	}

	encoded := ""
	if admin != nil {
		encoded = admin.PasswordHash
	}

	matched := verifyPassword(encoded, password)
	if !matched || admin == nil || strings.ToLower(strings.TrimSpace(username)) != admin.Username {
		return database.AdminSession{}, "", ErrAuthentication
	}

	session, raw := newAdminSession(admin.Version, s.now())
	if err := s.store.CreateSession(ctx, session, tokenHash(previous), s.now()); err != nil {
		return database.AdminSession{}, "", err
	}

	return session, raw, nil
}

// Session validates an opaque browser credential against persisted expiry and revocation.
func (s *AuthService) Session(ctx context.Context, token string) (database.AdminSession, string, error) {
	if !validAuthToken(token) {
		return database.AdminSession{}, "", ErrAuthentication
	}

	return s.store.Session(ctx, tokenHash(token), s.now())
}

// Logout revokes the current browser's session.
func (s *AuthService) Logout(ctx context.Context, token string) error {
	return s.store.Revoke(ctx, tokenHash(token))
}

// ResetPassword preserves the username and invalidates every active session.
func (s *AuthService) ResetPassword(ctx context.Context, password string) error {
	if !validAdminInput("admin", password) {
		return ErrAuthInput
	}

	encoded, err := passwordHash(password)
	if err != nil {
		return fmt.Errorf("hash replacement password: %w", err)
	}

	return s.store.ResetPassword(ctx, encoded)
}
