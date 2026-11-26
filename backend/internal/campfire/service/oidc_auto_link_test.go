package service

import (
	"errors"
	"slices"
	"testing"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/permissions"
)

func TestOIDCFirstSignInUsesPreassignedServerAccess(t *testing.T) {
	_, db, _, _ := activityFixture(t)
	ctx := t.Context()
	auth := NewAuthService(repository.NewAuthRepository(db.ORM()))

	input := CreateUserInput{Username: "future-player", Email: "Élodie@EXAMPLE.test", RoleID: database.UserRole}
	if _, err := auth.Access().CreateUser(ctx, input); !errors.Is(err, ErrAuthInput) {
		t.Fatal("passwordless account provisioned without automatic linking", err)
	}

	p := newTestOIDCProvider(t)

	provider, err := NewOIDCService(ctx, auth, OIDCConfig{Issuer: p.server.URL, ClientID: "campfire", ClientSecret: "secret", PublicURL: "http://localhost:8080", AutoLinkEmail: true})
	if err != nil {
		t.Fatal(err)
	}

	access := auth.Access()

	user, err := access.CreateUser(ctx, input)
	if err != nil || user.PasswordHash != "" {
		t.Fatal("pre-provisioning failed", err)
	}

	if _, _, err := auth.Login(ctx, user.Username, "", ""); !errors.Is(err, ErrAuthentication) {
		t.Fatal("accepted local login without a password", err)
	}

	if err := access.GrantServer(ctx, Principal{Admin: true, UserID: 1}, "game", AccessGrantInput{Username: user.Username, AccessRoleID: database.OperatorRole}); err != nil {
		t.Fatal(err)
	}

	cookie, state := p.flow(t, provider, 0, "player-subject", map[string]any{"email": "Élodie@example.test", "email_verified": true})

	session, _, err := provider.Complete(ctx, cookie, state, "valid")
	if err != nil || session.UserID != user.ID || session.User.RoleID != user.RoleID || session.User.Username != user.Username {
		t.Fatal("first login did not claim the assigned account", session.UserID, err)
	}

	grants, err := access.ServerPermissions(ctx, PrincipalFromSession(session), "game")
	if err != nil || !slices.Contains(grants, permissions.ServerOperate) || slices.Contains(grants, permissions.ServerDelete) {
		t.Fatal("incorrect server permissions on first login", grants, err)
	}
}

func TestSelfEditedEmailCannotAuthorizeOIDCLink(t *testing.T) {
	auth, _, raw := mfaFixture(t)
	address := "self-edited@example.test"

	local, err := auth.UpdateProfile(t.Context(), raw, nil, &address, "", "", "")
	if err != nil {
		t.Fatal(err)
	}

	input := repository.OIDCAccountInput{Issuer: "issuer", Subject: "first", Username: "new-user", Email: address, Options: repository.OIDCAccountOptions{AutoLinkEmail: true, EmailVerified: true}}

	user, err := auth.store.OIDCAccount(t.Context(), input)
	if err != nil || user.ID == local.ID || user.RoleID != database.UserRole {
		t.Fatal("self-edited address granted access to the local account", user.ID, err)
	}

	input.Subject, input.Email = "second", "admin@example.test"

	user, err = auth.store.OIDCAccount(t.Context(), input)
	if err != nil || user.ID == local.ID {
		t.Fatal("old address retained linking authority", user.ID, err)
	}
}

func TestPendingOIDCAdminCannotReplaceLastAdministrator(t *testing.T) {
	auth, _, p, config := oidcFixture(t)
	ctx := t.Context()
	config.AutoLinkEmail = true

	provider, err := NewOIDCService(ctx, auth, config)
	if err != nil {
		t.Fatal(err)
	}

	access := auth.Access()

	user, err := access.CreateUser(ctx, CreateUserInput{Username: "next-admin", Email: "next@example.test", RoleID: database.AdminRole})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := access.UpdateUser(ctx, 1, nil, new(true), "", nil, nil); !errors.Is(err, repository.ErrLastAdmin) {
		t.Fatal("unclaimed passwordless admin allowed disabling the last administrator", err)
	}

	cookie, state := p.flow(t, provider, 0, "next-admin-subject", map[string]any{"email": user.Email, "email_verified": true})

	session, _, err := provider.Complete(ctx, cookie, state, "valid")
	if err != nil || session.UserID != user.ID {
		t.Fatal("admin claim failed", err)
	}

	if _, err := access.UpdateUser(ctx, 1, nil, new(true), "", nil, nil); err != nil {
		t.Fatal("linked administrator was not counted", err)
	}
}

func TestOIDCAutoLinkEmailClaims(t *testing.T) {
	for _, test := range []struct {
		name       string
		enabled    bool
		claims     map[string]any
		profile    map[string]any
		linkID     int64
		wantLink   bool
		wantReject bool
	}{
		{name: "disabled by default", claims: map[string]any{"email_verified": true}},
		{name: "verified token email", enabled: true, claims: map[string]any{"email": " admin@EXAMPLE.com ", "email_verified": true}, wantLink: true},
		{name: "different local-part case has its own owner", enabled: true, claims: map[string]any{"email": "Admin@example.com", "email_verified": true}},
		{name: "missing verification", enabled: true, wantReject: true},
		{name: "unverified email", enabled: true, claims: map[string]any{"email_verified": false}, wantReject: true},
		{name: "no matching email", enabled: true, claims: map[string]any{"email": "new@example.com"}},
		{name: "new unverified provider email", enabled: true, claims: map[string]any{"email": "new@example.com", "email_verified": false}},
		{name: "available username with verified email match", enabled: true, claims: map[string]any{"preferred_username": "available", "email_verified": true}, wantLink: true},
		{name: "username collision with different verified email", enabled: true, claims: map[string]any{"email": "other@example.com", "email_verified": true}},
		{name: "new account requires email", enabled: true, claims: map[string]any{"email": ""}, wantReject: true},
		{name: "explicit link needs no verification", enabled: true, linkID: 1, wantLink: true},
		{name: "verified userinfo", enabled: true, claims: map[string]any{"email": "other@example.com"}, profile: map[string]any{"email": "admin@example.com", "email_verified": true}, wantLink: true},
		{name: "different userinfo email loses verification", enabled: true, claims: map[string]any{"email": "other@example.com", "email_verified": true}, profile: map[string]any{"email": "admin@example.com"}, wantReject: true},
		{name: "same userinfo email retains verification", enabled: true, claims: map[string]any{"email_verified": true}, profile: map[string]any{"email": "admin@EXAMPLE.com"}, wantLink: true},
		{name: "userinfo local-part change needs separate proof", enabled: true, claims: map[string]any{"email_verified": true}, profile: map[string]any{"email": "Admin@example.com"}},
		{name: "userinfo can revoke verification", enabled: true, claims: map[string]any{"email_verified": true}, profile: map[string]any{"email": "admin@example.com", "email_verified": false}, wantReject: true},
		{name: "userinfo verification needs its email", enabled: true, profile: map[string]any{"email_verified": true}, wantReject: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			auth, db, p, config := oidcFixture(t)
			if err := db.ORM().Model(&database.User{}).Where("id = ?", 1).Updates(map[string]any{"email": "admin@example.com", "oidc_link_email": "admin@example.com", "password_hash": "local-password-hash"}).Error; err != nil {
				t.Fatal(err)
			}

			config.AutoLinkEmail = test.enabled

			provider, err := NewOIDCService(t.Context(), auth, config)
			if err != nil {
				t.Fatal(err)
			}

			p.profile = test.profile
			cookie, state := p.flow(t, provider, test.linkID, "first-login", test.claims)

			session, _, err := provider.Complete(t.Context(), cookie, state, "valid")
			if test.wantReject {
				if err == nil {
					t.Fatal("unsafe email match accepted")
				}

				var users, identities int64
				if err := db.ORM().Model(&database.User{}).Count(&users).Error; err != nil {
					t.Fatal(err)
				}

				if err := db.ORM().Model(&database.OIDCIdentity{}).Count(&identities).Error; err != nil {
					t.Fatal(err)
				}

				if users != 1 || identities != 0 {
					t.Fatal("rejected login changed accounts", users, identities)
				}

				return
			}

			if err != nil {
				t.Fatal(err)
			}

			if test.wantLink {
				if session.UserID != 1 || session.User.Username != "admin" || session.User.RoleID != database.AdminRole || session.User.PasswordHash != "local-password-hash" {
					t.Fatal("link did not preserve the local account", session.User)
				}
			} else if session.UserID == 1 || session.User.RoleID != database.UserRole || session.User.PasswordHash != "" {
				t.Fatal("unmatched login selected or elevated an account", session.User)
			}
		})
	}
}

func TestOIDCAutoLinkPreservesSubjectBinding(t *testing.T) {
	auth, db, p, config := oidcFixture(t)
	if err := db.ORM().Model(&database.User{}).Where("id = ?", 1).Updates(map[string]any{"email": "admin@example.com", "oidc_link_email": "admin@example.com", "password_hash": "local-password-hash"}).Error; err != nil {
		t.Fatal(err)
	}

	config.AutoLinkEmail = true
	config.DisableLocal = true

	provider, err := NewOIDCService(t.Context(), auth, config)
	if err != nil {
		t.Fatal(err)
	}

	cookie, state := p.flow(t, provider, 0, "stable-subject", map[string]any{"email_verified": true})
	if session, _, err := provider.Complete(t.Context(), cookie, state, "valid"); err != nil || session.UserID != 1 {
		t.Fatal("initial automatic link failed", session.UserID, err)
	}

	if local, err := auth.LocalLoginEnabled(t.Context()); err != nil || local {
		t.Fatal("automatic admin link did not enable OIDC-only login", local, err)
	}

	other := database.User{Username: "other", Email: "other@example.com", PasswordHash: "other-password", CredentialVersion: 1, RoleID: database.UserRole}
	if err := db.ORM().Omit("Role").Create(&other).Error; err != nil {
		t.Fatal(err)
	}

	cookie, state = p.flow(t, provider, 0, "stable-subject", map[string]any{"email": other.Email})
	if session, _, err := provider.Complete(t.Context(), cookie, state, "valid"); err != nil || session.UserID != 1 {
		t.Fatal("email change moved an existing identity", session.UserID, err)
	}
}
