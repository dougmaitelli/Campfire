package repository_test

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/limits"
	"campfire/internal/shared/utils"
)

func TestOIDCProvisioningAfterFirstSignIn(t *testing.T) {
	db, store, _ := authStore(t)
	ctx := t.Context()
	admin := database.User{Username: "admin", RoleID: database.AdminRole}
	if err := db.ORM().Omit("Role").Create(&admin).Error; err != nil {
		t.Fatal(err)
	}
	access := store.Access().WithLoginPolicy("issuer", false, true)
	input := repository.OIDCAccountInput{Issuer: "issuer", Subject: "player-subject", Username: "returning-player", Email: "Élodie@bücher.test", Options: repository.OIDCAccountOptions{AutoLinkEmail: true, EmailVerified: true}}
	player, err := store.OIDCAccount(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	// Profiles from before schema 12 can retain the provider's domain spelling.
	if err := db.ORM().Model(&player).Update("email", "Élodie@BÜCHER.test").Error; err != nil {
		t.Fatal(err)
	}
	address := utils.NormalizeEmail(input.Email)
	pending := database.User{Username: "future-player", RoleID: database.CreatorRole, Email: address, OIDCLinkEmail: &address}
	if err := access.CreateUser(ctx, &pending); !errors.Is(err, repository.ErrAccessConflict) || !strings.Contains(err.Error(), player.Username) {
		t.Fatal("expected a conflict identifying the existing account", err)
	}

	other := utils.NormalizeEmail("élodie@bücher.test")
	pending.Email, pending.OIDCLinkEmail = other, &other
	if err := access.CreateUser(ctx, &pending); err != nil {
		t.Fatal("distinct mailbox should be available", err)
	}
	if err := access.UpdateUser(ctx, pending.ID, nil, nil, "", nil, &address); !errors.Is(err, repository.ErrAccessConflict) {
		t.Fatal("could reassign the existing player's email to a pending account", err)
	}
	saved, err := access.User(ctx, pending.ID)
	if err != nil || saved.Email != other || saved.OIDCLinkEmail == nil || *saved.OIDCLinkEmail != other {
		t.Fatal("failed assignment changed the reservation", err)
	}
	returning, err := store.OIDCAccount(ctx, input)
	if err != nil || returning.ID != player.ID || returning.RoleID != database.UserRole {
		t.Fatal("provisioning changed the returning player's access", err)
	}
}

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

			input := repository.OIDCAccountInput{Issuer: "issuer", Subject: "player-subject", Username: preferred, Email: "player@example.test"}

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

			user, err := store.OIDCAccount(t.Context(), repository.OIDCAccountInput{Issuer: "issuer", Subject: []string{"first", "second"}[i], Username: "player", Email: "player@example.test"})
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
	for _, scenario := range []string{"disabled", "already linked"} {
		t.Run(scenario, func(t *testing.T) {
			db, store, _ := authStore(t)

			user := database.User{Username: "local", Email: "person@example.com", PasswordHash: "encoded", CredentialVersion: 1, RoleID: database.UserRole}
			if scenario == "disabled" {
				user.Disabled = true
			}

			user.OIDCLinkEmail = &user.Email

			if err := db.ORM().Omit("Role").Create(&user).Error; err != nil {
				t.Fatal(err)
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
	testOIDCLinkRaceAcrossConnections(t, false)
}

func TestOIDCExplicitLinkRaceAcrossConnections(t *testing.T) {
	testOIDCLinkRaceAcrossConnections(t, true)
}

func testOIDCLinkRaceAcrossConnections(t *testing.T, explicit bool) {
	t.Helper()
	db, first, source := authStore(t)

	user := database.User{Username: "local", Email: "Élodie@example.com", PasswordHash: "encoded", CredentialVersion: 3, RoleID: database.CreatorRole}

	user.OIDCLinkEmail = &user.Email
	if err := db.ORM().Omit("Role").Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	now := time.Now()

	session := database.UserSession{Hash: "link-session", UserID: user.ID, CredentialVersion: user.CredentialVersion, ExpiresAt: now.Add(time.Hour).Unix(), LastSeenAt: now.Unix()}
	if explicit {
		if err := db.ORM().Omit("User").Create(&session).Error; err != nil {
			t.Fatal(err)
		}
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

			input := repository.OIDCAccountInput{Issuer: "issuer", Subject: []string{"first", "second"}[i], Username: "unused", Email: "Élodie@EXAMPLE.com", Options: repository.OIDCAccountOptions{AutoLinkEmail: !explicit, EmailVerified: true}}
			if explicit {
				input.LinkUserID, input.CredentialVersion, input.LinkSessionHash = user.ID, user.CredentialVersion, session.Hash
				input.Email = input.Subject + "@example.com"
			}

			linked, err := store.OIDCAccount(t.Context(), input)
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

	if explicit {
		var identities []database.OIDCIdentity
		if err := db.ORM().Where("user_id = ?", user.ID).Find(&identities).Error; err != nil || len(identities) != 1 {
			t.Fatal("expected one active identity", identities, err)
		}

		winner := identities[0].Subject

		var stored database.User
		if err := db.ORM().Take(&stored, user.ID).Error; err != nil || stored.Email != winner+"@example.com" {
			t.Fatal("competing subject changed profile", stored.Email, err)
		}

		if _, err := first.OIDCAccount(t.Context(), repository.OIDCAccountInput{Issuer: "issuer", Subject: winner, LinkUserID: user.ID, LinkSessionHash: session.Hash, CredentialVersion: user.CredentialVersion}); err != nil {
			t.Fatal("relinking the same identity failed", err)
		}
	}
}
