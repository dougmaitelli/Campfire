package service

import (
	"testing"

	"campfire/internal/campfire/database"
)

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
		{name: "verified token email", enabled: true, claims: map[string]any{"email": " ADMIN@example.com ", "email_verified": true}, wantLink: true},
		{name: "missing verification", enabled: true, wantReject: true},
		{name: "unverified email", enabled: true, claims: map[string]any{"email_verified": false}, wantReject: true},
		{name: "no matching email", enabled: true, claims: map[string]any{"email": "new@example.com"}},
		{name: "available username with verified email match", enabled: true, claims: map[string]any{"preferred_username": "available", "email_verified": true}, wantLink: true},
		{name: "username collision with different verified email", enabled: true, claims: map[string]any{"email": "other@example.com", "email_verified": true}},
		{name: "no email", enabled: true, claims: map[string]any{"email": ""}},
		{name: "explicit link needs no verification", enabled: true, linkID: 1, wantLink: true},
		{name: "verified userinfo", enabled: true, claims: map[string]any{"email": "other@example.com"}, profile: map[string]any{"email": "admin@example.com", "email_verified": true}, wantLink: true},
		{name: "different userinfo email loses verification", enabled: true, claims: map[string]any{"email": "other@example.com", "email_verified": true}, profile: map[string]any{"email": "admin@example.com"}, wantReject: true},
		{name: "same userinfo email retains verification", enabled: true, claims: map[string]any{"email_verified": true}, profile: map[string]any{"email": "ADMIN@example.com"}, wantLink: true},
		{name: "userinfo can revoke verification", enabled: true, claims: map[string]any{"email_verified": true}, profile: map[string]any{"email": "admin@example.com", "email_verified": false}, wantReject: true},
		{name: "userinfo verification needs its email", enabled: true, profile: map[string]any{"email_verified": true}, wantReject: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			auth, db, p, config := oidcFixture(t)
			if err := db.ORM().Model(&database.User{}).Where("id = ?", 1).Updates(map[string]any{"email": "admin@example.com", "password_hash": "local-password-hash"}).Error; err != nil {
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
	if err := db.ORM().Model(&database.User{}).Where("id = ?", 1).Updates(map[string]any{"email": "admin@example.com", "password_hash": "local-password-hash"}).Error; err != nil {
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

	if enabled, err := auth.LocalLoginEnabled(t.Context()); err != nil || enabled {
		t.Fatal("automatic admin link did not enable OIDC-only policy", enabled, err)
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
