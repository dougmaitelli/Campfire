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

	p.profile = map[string]any{"name": "Provider Name", "email": "profile@example.com", "picture": "https://example.com/avatar.png"}
	cookie, state := p.flow(t, provider, 0, "profile-user", map[string]any{"email": ""})

	session, _, err := provider.Complete(t.Context(), cookie, state, "valid")
	if err != nil {
		t.Fatal(err)
	}

	user := session.User
	if user.Name != "Provider Name" || user.Email != "profile@example.com" || !user.OIDCNameManaged || !user.OIDCEmailManaged {
		t.Fatal("UserInfo profile not synchronized", user)
	}

	access := repository.NewAccessRepository(db.ORM())

	changed := "Local override"
	if err := access.UpdateUser(t.Context(), user.ID, user.RoleID, false, "", &changed, nil); err != nil {
		t.Fatal("admin could not edit an OIDC user's name", err)
	}

	changedEmail := "override@example.com"
	if err := access.UpdateUser(t.Context(), user.ID, user.RoleID, false, "", nil, &changedEmail); err != nil {
		t.Fatal("admin could not edit an OIDC user's email", err)
	}

	edited, err := access.User(t.Context(), user.ID)
	if err != nil || edited.Name != changed || edited.Email != changedEmail {
		t.Fatal("admin profile edits did not persist", edited, err)
	}

	p.profile = map[string]any{"name": "Updated Name", "email": "updated@example.com"}
	cookie, state = p.flow(t, provider, 0, "profile-user", nil)

	updated, _, err := provider.Complete(t.Context(), cookie, state, "valid")
	if err != nil || updated.UserID != user.ID || updated.User.Username != user.Username || updated.User.Name != "Updated Name" || updated.User.Email != "updated@example.com" {
		t.Fatal("repeat login failed to synchronize profile with stable identity", updated.User, err)
	}

	p.profile = nil
	cookie, state = p.flow(t, provider, 0, "profile-user", map[string]any{"email": ""})

	missing, _, err := provider.Complete(t.Context(), cookie, state, "valid")
	if err != nil || missing.User.OIDCNameManaged || missing.User.OIDCEmailManaged || missing.User.Name != "Updated Name" || missing.User.Email != "updated@example.com" {
		t.Fatal("missing claims should preserve values and allow local editing", missing.User, err)
	}

	if err := access.UpdateUser(t.Context(), user.ID, user.RoleID, false, "", &changed, nil); err != nil {
		t.Fatal("unmanaged name was not editable", err)
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
