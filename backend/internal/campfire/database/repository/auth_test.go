package repository_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
)

func authStore(t *testing.T) (*database.Database, *repository.AuthRepository, string) {
	t.Helper()

	path := testDatabaseSource(t)

	db, err := database.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	return db, repository.NewAuthRepository(db.ORM()), path
}

func TestAdministratorSetupRaceAcrossConnections(t *testing.T) {
	_, first, path := authStore(t)

	secondDB, err := database.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = secondDB.Close() }()

	second := repository.NewAuthRepository(secondDB.ORM())
	results := make(chan error, 2)

	var wait sync.WaitGroup
	for _, store := range []*repository.AuthRepository{first, second} {
		wait.Add(1)
		go func() {
			defer wait.Done()

			results <- store.Setup(context.Background(), database.User{Username: "admin", PasswordHash: "encoded", Version: 1}, database.UserSession{Hash: "initial", CSRF: "csrf", CredentialVersion: 1, ExpiresAt: time.Now().Add(time.Hour).Unix(), LastSeenAt: time.Now().Unix()})
		}()
	}

	wait.Wait()
	close(results)

	var successes, conflicts int

	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, repository.ErrAdminExists) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}

	if successes != 1 || conflicts != 1 {
		t.Fatal("concurrent setup did not create exactly one admin")
	}
}

func TestSessionPersistenceExpiryAndPasswordResetGeneration(t *testing.T) {
	db, store, path := authStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	session := database.UserSession{UserID: 1, Hash: "hashed-session", CSRF: "csrf", CredentialVersion: 1, ExpiresAt: now.Add(12 * time.Hour).Unix(), LastSeenAt: now.Unix()}

	if err := store.Setup(ctx, database.User{Username: "admin", PasswordHash: "encoded", Version: 1}, session); err != nil {
		t.Fatal(err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := database.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()

	store = repository.NewAuthRepository(reopened.ORM())
	if got, username, err := store.Session(ctx, session.Hash, now.Add(time.Minute)); err != nil || username != "admin" || got.CSRF != session.CSRF {
		t.Fatal("session did not survive reopen", err)
	}

	if _, _, err := store.Session(ctx, session.Hash, now.Add(31*time.Minute)); !errors.Is(err, repository.ErrAuthentication) {
		t.Fatal("idle session remained valid", err)
	}

	if err := reopened.ORM().Model(&database.UserSession{}).Where("hash = ?", session.Hash).Update("last_seen_at", now.Add(12*time.Hour-time.Minute).Unix()).Error; err != nil {
		t.Fatal(err)
	}

	if _, _, err := store.Session(ctx, session.Hash, now.Add(12*time.Hour)); !errors.Is(err, repository.ErrAuthentication) {
		t.Fatal("absolute session expiry was ignored", err)
	}

	if err := store.ResetPassword(ctx, "replacement"); err != nil {
		t.Fatal(err)
	}

	if _, _, err := store.Session(ctx, session.Hash, now); !errors.Is(err, repository.ErrAuthentication) {
		t.Fatal("reset left an old session valid", err)
	}

	if err := store.CreateSession(ctx, session, "", now); !errors.Is(err, repository.ErrAuthentication) {
		t.Fatal("login racing password reset used stale credentials", err)
	}

	admin, err := store.Credential(ctx)
	if err != nil || admin == nil || admin.Version != 2 || admin.PasswordHash != "replacement" {
		t.Fatal("reset did not preserve and increment credential generation", err)
	}
}

func TestConcurrentAdminDemotionKeepsOneEnabledAdministrator(t *testing.T) {
	db, store, path := authStore(t)

	ctx := context.Background()
	if err := store.Setup(ctx, database.User{Username: "admin", PasswordHash: "encoded", Version: 1}, database.UserSession{Hash: "first", CSRF: "csrf", CredentialVersion: 1}); err != nil {
		t.Fatal(err)
	}

	other := database.User{Username: "second-admin", PasswordHash: "encoded", Version: 1, RoleID: database.AdminRole}
	if err := store.Access().CreateUser(ctx, &other); err != nil {
		t.Fatal(err)
	}

	second, err := database.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = second.Close() }()

	results := make(chan error, 2)

	var workers sync.WaitGroup
	for index, access := range []*repository.AccessRepository{store.Access(), repository.NewAccessRepository(second.ORM())} {
		workers.Add(1)

		id := int64(1)
		if index == 1 {
			id = other.ID
		}

		go func() { defer workers.Done(); results <- access.UpdateUser(ctx, id, database.UserRole, false, "", nil) }()
	}

	workers.Wait()
	close(results)

	var succeeded, prevented int

	for err := range results {
		if err == nil {
			succeeded++
		} else if errors.Is(err, repository.ErrLastAdmin) {
			prevented++
		} else {
			t.Fatal(err)
		}
	}

	if succeeded != 1 || prevented != 1 {
		t.Fatal("concurrent demotion did not protect the last administrator", succeeded, prevented)
	}

	var admins int64
	if err := db.ORM().Model(&database.User{}).Where("role_id = ? AND disabled = false", database.AdminRole).Count(&admins).Error; err != nil || admins != 1 {
		t.Fatal("invalid active administrator count", admins, err)
	}
}
