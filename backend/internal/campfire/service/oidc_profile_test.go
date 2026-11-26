package service

import (
	"errors"
	"testing"

	"campfire/internal/campfire/database/repository"
)

func TestOIDCUserInfoProfileManagement(t *testing.T) {
	auth, db, p, config := oidcFixture(t)

	provider, err := NewOIDCService(t.Context(), auth, config)
	if err != nil {
		t.Fatal(err)
	}

	p.profile = map[string]any{"name": "Provider Name", "email": "Profile@example.com", "email_verified": true, "picture": "https://example.com/avatar.png"}
	cookie, state := p.flow(t, provider, 0, "profile-user", map[string]any{"email": ""})

	session, _, err := provider.Complete(t.Context(), cookie, state, "valid")
	if err != nil {
		t.Fatal(err)
	}

	user := session.User
	if user.Name != "Provider Name" || user.Email != "Profile@example.com" || !user.OIDCNameManaged || !user.OIDCEmailManaged {
		t.Fatal("UserInfo profile not synchronized", user)
	}

	access := repository.NewAccessRepository(db.ORM())

	changed := "Local override"
	if err := access.UpdateUser(t.Context(), user.ID, nil, nil, "", &changed, nil); !errors.Is(err, repository.ErrAccessDenied) {
		t.Fatal("admin could edit an OIDC user's name", err)
	}

	changedEmail := "override@example.com"
	if err := access.UpdateUser(t.Context(), user.ID, nil, nil, "", nil, &changedEmail); !errors.Is(err, repository.ErrAccessDenied) {
		t.Fatal("admin could edit an OIDC user's email", err)
	}

	edited, err := access.User(t.Context(), user.ID)
	if err != nil || edited.Name != user.Name || edited.Email != user.Email {
		t.Fatal("rejected edits changed profile", edited, err)
	}

	p.profile = map[string]any{"name": "Updated Name", "email": "updated@example.com"}
	cookie, state = p.flow(t, provider, 0, "profile-user", nil)

	updated, _, err := provider.Complete(t.Context(), cookie, state, "valid")
	if err != nil || updated.UserID != user.ID || updated.User.Username != user.Username || updated.User.Name != "Updated Name" || updated.User.Email != "updated@example.com" {
		t.Fatal("repeat login failed to synchronize profile with stable identity", updated.User, err)
	}

	p.profile = nil
	cookie, state = p.flow(t, provider, 0, "profile-user", map[string]any{"email": ""})

	missing, raw, err := provider.Complete(t.Context(), cookie, state, "valid")
	if err != nil || !missing.User.OIDCNameManaged || !missing.User.OIDCEmailManaged || missing.User.Name != "Updated Name" || missing.User.Email != "updated@example.com" {
		t.Fatal("missing claims should preserve externally managed values", missing.User, err)
	}

	if _, err := auth.UpdateProfile(t.Context(), raw, &changed, nil, "", "", ""); !errors.Is(err, repository.ErrAccessDenied) {
		t.Fatal("missing name claim allowed local editing", err)
	}

	if _, err := auth.UpdateProfile(t.Context(), raw, nil, &changedEmail, "", "", ""); !errors.Is(err, repository.ErrAccessDenied) {
		t.Fatal("missing email claim allowed local editing", err)
	}

	p.profile = map[string]any{"sub": "another-user", "name": "Wrong User"}

	cookie, state = p.flow(t, provider, 0, "profile-user", nil)
	if _, _, err := provider.Complete(t.Context(), cookie, state, "valid"); !errors.Is(err, ErrAuthentication) {
		t.Fatal("mismatched UserInfo subject accepted", err)
	}

	p.profile = nil

	if err := db.ORM().Table("users").Where("id = ?", 1).Update("name", "Existing Local Name").Error; err != nil {
		t.Fatal(err)
	}

	cookie, state = p.flow(t, provider, 1, "linked-admin", map[string]any{"name": "Token Admin", "email": "token@example.com"})

	linked, _, err := provider.Complete(t.Context(), cookie, state, "valid")
	if err != nil || linked.UserID != 1 || linked.User.Name != "Token Admin" || linked.User.Email != "token@example.com" || !linked.User.OIDCNameManaged || !linked.User.OIDCEmailManaged {
		t.Fatal("linking did not import ID-token profile", linked.User, err)
	}
}

func TestLinkProtectsExistingLocalSessions(t *testing.T) {
	auth, _, raw := mfaFixture(t)

	const password = "long test password for two factors A1!"

	session, _, err := auth.Session(t.Context(), raw)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := auth.store.OIDCAccount(t.Context(), repository.OIDCAccountInput{
		Issuer: "issuer", Subject: "linked", LinkUserID: session.UserID, LinkSessionHash: session.Hash,
		CredentialVersion: session.CredentialVersion, Email: session.User.Email,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := auth.UpdateProfile(t.Context(), raw, new("Override"), nil, "", "", ""); !errors.Is(err, repository.ErrAccessDenied) {
		t.Fatal("linked password session could edit the name", err)
	}

	if _, err := auth.UpdateProfile(t.Context(), raw, nil, nil, "Replacement password1!", password, ""); err != nil {
		t.Fatal("linked password session could not change its password", err)
	}
}
