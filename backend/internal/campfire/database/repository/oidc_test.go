package repository_test

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/limits"
	"campfire/internal/shared/utils"
)

func TestOIDCUsernameAvailability(t *testing.T) {
	for _, scenario := range []string{"available", "collision", "case insensitive", "deleted account", "maximum length"} {
		t.Run(scenario, func(t *testing.T) {
			db, store, _ := authStore(t)

			admin := database.User{Username: "admin", CredentialVersion: 1, RoleID: database.AdminRole}
			if err := db.ORM().Omit("Role").Create(&admin).Error; err != nil {
				t.Fatal(err)
			}

			preferred := "player"
			if scenario == "maximum length" {
				preferred = strings.Repeat("a", limits.UsernameMax)
			}

			existing := database.User{Username: preferred, PasswordHash: "local-password", CredentialVersion: 1, RoleID: database.UserRole}
			if scenario == "case insensitive" {
				existing.Username = strings.ToUpper(preferred)
			}

			if scenario != "available" {
				if err := db.ORM().Omit("Role").Create(&existing).Error; err != nil {
					t.Fatal(err)
				}
			}

			if scenario == "deleted account" {
				if err := db.ORM().Delete(&existing).Error; err != nil {
					t.Fatal(err)
				}
			}

			input := repository.OIDCAccountInput{Issuer: "issuer", Subject: "player-subject", Username: preferred}

			user, err := store.OIDCAccount(t.Context(), input)
			if err != nil {
				t.Fatal(err)
			}

			if !utils.IsValidUsername(user.Username) || user.ID == existing.ID || user.PasswordHash != "" || user.RoleID != database.UserRole {
				t.Fatal("invalid provisioned account", user)
			}

			if scenario == "available" {
				if user.Username != preferred {
					t.Fatal("available username received a suffix", user.Username)
				}
			} else {
				base, suffix, found := strings.Cut(user.Username, "-")
				if !found || base == "" || suffix == "" || !strings.HasPrefix(preferred, base) || user.Username == preferred {
					t.Fatal("colliding username did not receive a suffix", user.Username)
				}
			}

			input.Username = "changed"

			repeated, err := store.OIDCAccount(t.Context(), input)
			if err != nil || repeated.ID != user.ID || repeated.Username != user.Username {
				t.Fatal("existing identity did not retain its username", repeated, err)
			}
		})
	}
}

func TestOIDCUsernameRaceAcrossConnections(t *testing.T) {
	db, first, source := authStore(t)

	admin := database.User{Username: "admin", CredentialVersion: 1, RoleID: database.AdminRole}
	if err := db.ORM().Omit("Role").Create(&admin).Error; err != nil {
		t.Fatal(err)
	}

	secondDB, err := database.Open(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = secondDB.Close() }()

	type result struct {
		user database.User
		err  error
	}

	results := make(chan result, 2)
	start := make(chan struct{})

	var wait sync.WaitGroup
	for i, store := range []*repository.AuthRepository{first, repository.NewAuthRepository(secondDB.ORM())} {
		wait.Go(func() {
			<-start

			user, err := store.OIDCAccount(t.Context(), repository.OIDCAccountInput{Issuer: "issuer", Subject: []string{"first", "second"}[i], Username: "player"})
			results <- result{user: user, err: err}
		})
	}

	close(start)
	wait.Wait()
	close(results)

	usernames := make(map[string]bool)

	for result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}

		if !utils.IsValidUsername(result.user.Username) || usernames[result.user.Username] {
			t.Fatal("concurrent login reused a username", result.user.Username)
		}

		usernames[result.user.Username] = true
	}

	if len(usernames) != 2 || !usernames["player"] {
		t.Fatal("concurrent logins did not retain one preferred username", usernames)
	}
}

func TestOIDCAutoLinkRejectsUnsafeAccounts(t *testing.T) {
	for _, scenario := range []string{"ambiguous", "disabled", "passwordless", "already linked"} {
		t.Run(scenario, func(t *testing.T) {
			db, store, _ := authStore(t)

			user := database.User{Username: "local", Email: "person@example.com", PasswordHash: "encoded", CredentialVersion: 1, RoleID: database.UserRole}
			if scenario == "disabled" {
				user.Disabled = true
			}

			if scenario == "passwordless" {
				user.PasswordHash = ""
			}

			if err := db.ORM().Omit("Role").Create(&user).Error; err != nil {
				t.Fatal(err)
			}

			if scenario == "ambiguous" {
				other := database.User{Username: "duplicate", Email: " PERSON@EXAMPLE.COM ", PasswordHash: "encoded", CredentialVersion: 1, RoleID: database.UserRole}
				if err := db.ORM().Omit("Role").Create(&other).Error; err != nil {
					t.Fatal(err)
				}
			}

			if scenario == "already linked" {
				if err := db.ORM().Omit("User").Create(&database.OIDCIdentity{UserID: user.ID, Issuer: "another-issuer", Subject: "existing"}).Error; err != nil {
					t.Fatal(err)
				}
			}

			_, err := store.OIDCAccount(t.Context(), repository.OIDCAccountInput{Issuer: "issuer", Subject: "unknown", Username: "new-user", LinkUserID: 0, CredentialVersion: 0, LinkSessionHash: "", Name: "", Email: user.Email, Picture: "", Options: repository.OIDCAccountOptions{AutoLinkEmail: true, EmailVerified: true}})
			if !errors.Is(err, repository.ErrAuthentication) && !errors.Is(err, repository.ErrAccessConflict) {
				t.Fatal("unsafe account match was not rejected", err)
			}

			var identities int64
			if err := db.ORM().Model(&database.OIDCIdentity{}).Where("issuer = ?", "issuer").Count(&identities).Error; err != nil || identities != 0 {
				t.Fatal("rejected login created an identity", identities, err)
			}
		})
	}
}

func TestOIDCAutoLinkRaceAcrossConnections(t *testing.T) {
	db, first, source := authStore(t)

	user := database.User{Username: "local", Email: "person@example.com", PasswordHash: "encoded", CredentialVersion: 3, RoleID: database.CreatorRole}
	if err := db.ORM().Omit("Role").Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	secondDB, err := database.Open(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = secondDB.Close() }()

	second := repository.NewAuthRepository(secondDB.ORM())
	results := make(chan error, 2)
	start := make(chan struct{})

	var wait sync.WaitGroup
	for i, store := range []*repository.AuthRepository{first, second} {
		wait.Add(1)
		go func() {
			defer wait.Done()

			<-start

			linked, err := store.OIDCAccount(t.Context(), repository.OIDCAccountInput{Issuer: "issuer", Subject: []string{"first", "second"}[i], Username: "unused", LinkUserID: 0, CredentialVersion: 0, LinkSessionHash: "", Name: "", Email: "PERSON@example.com", Picture: "", Options: repository.OIDCAccountOptions{AutoLinkEmail: true, EmailVerified: true}})
			if err == nil && (linked.ID != user.ID || linked.RoleID != user.RoleID || linked.CredentialVersion != user.CredentialVersion || linked.PasswordHash != user.PasswordHash) {
				t.Error("automatic link changed account identity or credentials", linked)
			}

			results <- err
		}()
	}

	close(start)
	wait.Wait()
	close(results)

	var successes, conflicts int

	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, repository.ErrAccessConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}

	if successes != 1 || conflicts != 1 {
		t.Fatal("concurrent identities both claimed the account", successes, conflicts)
	}
}
