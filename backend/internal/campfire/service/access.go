package service

import (
	"context"
	"net/mail"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
)

// Principal is derived from persisted credentials, never from client-supplied roles.
type Principal struct {
	APITokenID        int64    `json:"api_token_id,omitempty"`
	TokenScopes       []string `json:"-"`
	CredentialVersion int64    `json:"-"`
	UserID            int64    `json:"user_id"`
	Username          string   `json:"username"`
	Name              string   `json:"name"`
	Picture           string   `json:"picture"`
	RoleID            int64    `json:"role_id"`
	Role              string   `json:"role"`
	Admin             bool     `json:"admin"`
	Permissions       []string `json:"permissions"`
}

// RoleView exposes action names without ORM association details.
type RoleView struct {
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	Scope       string   `json:"scope"`
	Builtin     bool     `json:"builtin"`
	Permissions []string `json:"permissions"`
}

// PrincipalFromSession uses the account and permission set freshly loaded during authentication.
func PrincipalFromSession(session database.UserSession) Principal {
	return Principal{UserID: session.UserID, Username: session.User.Username, Name: session.User.Name, Picture: session.User.Picture, RoleID: session.User.RoleID, Role: session.User.Role.Name, Admin: session.User.RoleID == database.AdminRole, Permissions: roleActions(session.User.Role), CredentialVersion: session.User.Version}
}

// Allows checks a platform action against the user's permissions and token scopes.
func (p Principal) Allows(action string) bool {
	return p.tokenAllows(action) && (p.Admin || slices.Contains(p.Permissions, action))
}

func (p Principal) tokenAllows(action string) bool {
	return p.APITokenID == 0 || slices.Contains(p.TokenScopes, "*") || slices.Contains(p.TokenScopes, action)
}

func (p Principal) restrictServer(actions []string) []string {
	if p.APITokenID == 0 {
		return actions
	}

	return slices.DeleteFunc(slices.Clone(actions), func(action string) bool { return !p.tokenAllows(action) })
}

func roleActions(role database.Role) []string {
	actions := make([]string, 0, len(role.Permissions))
	for _, permission := range role.Permissions {
		actions = append(actions, permission.Permission)
	}

	slices.Sort(actions)

	return actions
}

// AccessService owns role validation and resource-scoped permission decisions.
type AccessService struct {
	store *repository.AccessRepository
	auth  *AuthService
}

// Access returns authorization and account management for this authentication store.
func (s *AuthService) Access() *AccessService {
	store := s.store.Access()
	if s.disableLocal {
		store.WithRequiredOIDC(s.loginProvider)
	}

	return &AccessService{store: store, auth: s}
}

// ServerPermissions calculates effective access from ownership or a current membership.
func (s *AccessService) ServerPermissions(ctx context.Context, principal Principal, id string) ([]string, error) {
	owner, err := s.store.Owner(ctx, id)
	if err != nil {
		return nil, err
	}

	if principal.Admin || owner != nil && owner.ID == principal.UserID {
		role, err := s.store.Role(ctx, database.OwnerRole)
		return principal.restrictServer(roleActions(role)), err
	}

	role, err := s.store.MemberRole(ctx, id, principal.UserID)

	return principal.restrictServer(roleActions(role)), err
}

// Roles returns the built-in and custom role catalog.
func (s *AccessService) Roles(ctx context.Context) ([]RoleView, error) {
	roles, err := s.store.Roles(ctx)

	views := make([]RoleView, 0, len(roles))
	for _, role := range roles {
		views = append(views, RoleView{role.ID, role.Name, role.Scope, role.Builtin, roleActions(role)})
	}

	return views, err
}

// SaveRole permits platform creation grants or server-scoped actions, never custom admin bypasses.
func (s *AccessService) SaveRole(ctx context.Context, id int64, name, scope string, permissions []string) (RoleView, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 64 || strings.ContainsFunc(name, unicode.IsControl) || scope != "platform" && scope != "server" {
		return RoleView{}, repository.ErrAccessInput
	}

	allowed := []string{"servers.create"}
	if scope == "server" {
		allowed = []string{"server.view", "server.operate", "server.configure", "server.delete", "server.share"}
	}

	role := database.Role{ID: id, Name: name, Scope: scope}

	slices.Sort(permissions)

	for _, permission := range slices.Compact(permissions) {
		if !slices.Contains(allowed, permission) {
			return RoleView{}, repository.ErrAccessInput
		}

		role.Permissions = append(role.Permissions, database.RolePermission{Permission: permission})
	}

	if scope == "server" && !slices.Contains(permissions, "server.view") {
		return RoleView{}, repository.ErrAccessInput
	}

	if err := s.store.SaveRole(ctx, &role); err != nil {
		return RoleView{}, err
	}

	return RoleView{role.ID, role.Name, role.Scope, role.Builtin, roleActions(role)}, nil
}

// DeleteRole removes an unused custom role.
func (s *AccessService) DeleteRole(ctx context.Context, id int64) error {
	return s.store.DeleteRole(ctx, id)
}

// Users lists accounts for administrators.
func (s *AccessService) Users(ctx context.Context) ([]database.User, error) {
	return s.store.Users(ctx)
}

// CreateUser validates and hashes a new account password.
func (s *AccessService) CreateUser(ctx context.Context, username, password string, roleID int64, disabled bool, name, email string) (database.User, error) {
	name = strings.TrimSpace(name)

	email = strings.TrimSpace(email)
	if !validDisplayName(name) || !validEmail(email) {
		return database.User{}, repository.ErrAccessInput
	}

	username = strings.ToLower(strings.TrimSpace(username))
	if !validAccountInput(username, password) {
		return database.User{}, ErrAuthInput
	}

	release, err := s.auth.passwordWork(ctx)
	if err != nil {
		return database.User{}, err
	}
	defer release()

	encoded, err := passwordHash(password)
	if err != nil {
		return database.User{}, err
	}

	user := database.User{Username: username, Name: name, Email: email, PasswordHash: encoded, Version: 1, RoleID: roleID, Disabled: disabled}
	if err := s.store.CreateUser(ctx, &user); err != nil {
		return database.User{}, err
	}

	return s.store.User(ctx, user.ID)
}

// UpdateUser changes account privileges and revokes that user's sessions.
func (s *AccessService) UpdateUser(ctx context.Context, id, roleID int64, disabled bool, password string, name, email *string) (database.User, error) {
	if name != nil {
		trimmed := strings.TrimSpace(*name)
		if !validDisplayName(trimmed) {
			return database.User{}, repository.ErrAccessInput
		}

		name = &trimmed
	}

	if email != nil {
		trimmed := strings.TrimSpace(*email)
		if !validEmail(trimmed) {
			return database.User{}, repository.ErrAccessInput
		}

		email = &trimmed
	}

	encoded := ""

	if password != "" {
		if !validAccountInput("admin", password) {
			return database.User{}, ErrAuthInput
		}

		release, err := s.auth.passwordWork(ctx)
		if err != nil {
			return database.User{}, err
		}
		defer release()

		encoded, err = passwordHash(password)
		if err != nil {
			return database.User{}, err
		}
	}

	if err := s.store.UpdateUser(ctx, id, roleID, disabled, encoded, name, email); err != nil {
		return database.User{}, err
	}

	return s.store.User(ctx, id)
}

func validDisplayName(name string) bool {
	return utf8.ValidString(name) && utf8.RuneCountInString(name) <= 100 && !strings.ContainsFunc(name, unicode.IsControl)
}

func validEmail(email string) bool {
	if email == "" {
		return true
	}

	if len(email) > 254 || !utf8.ValidString(email) || strings.ContainsFunc(email, unicode.IsControl) {
		return false
	}

	address, err := mail.ParseAddress(email)

	return err == nil && address.Address == email && address.Name == ""
}

// DeleteUser cannot remove accounts that still own servers or the last active administrator.
func (s *AccessService) DeleteUser(ctx context.Context, id int64) error {
	return s.store.DeleteUser(ctx, id)
}

// Owner validates an enabled account for server creation or ownership transfer.
func (s *AccessService) Owner(ctx context.Context, id int64) (database.User, error) {
	user, err := s.store.User(ctx, id)
	if err == nil && user.Disabled {
		return user, repository.ErrAccessInput
	}

	return user, err
}

// ServerAccess returns the current owner and explicit collaborator grants.
func (s *AccessService) ServerAccess(ctx context.Context, id string) (*database.User, []database.ServerMember, error) {
	owner, err := s.store.Owner(ctx, id)
	if err != nil {
		return nil, nil, err
	}

	members, err := s.store.Members(ctx, id)

	return owner, members, err
}

// Share lets owners add operators; only admins can grant a custom server role.
func (s *AccessService) Share(ctx context.Context, principal Principal, id, username string, roleID int64) error {
	if !principal.Admin && roleID != database.OperatorRole {
		return repository.ErrAccessDenied
	}

	user, err := s.auth.store.FindCredential(ctx, strings.ToLower(strings.TrimSpace(username)))
	if err != nil {
		return err
	}

	if user == nil || user.Disabled {
		return repository.ErrAccessInput
	}

	return s.store.SetMember(ctx, database.ServerMember{ServerID: id, UserID: user.ID, RoleID: roleID})
}

// RemoveMember revokes a collaborator without deleting the account.
func (s *AccessService) RemoveMember(ctx context.Context, id string, userID int64) error {
	return s.store.RemoveMember(ctx, id, userID)
}

// TransferOwner is exposed only to platform administrators.
func (s *AccessService) TransferOwner(ctx context.Context, id string, userID int64) error {
	return s.store.TransferOwner(ctx, id, userID)
}

// SparkRoles lists provisioning grants for administrators.
func (s *AccessService) SparkRoles(ctx context.Context, id string) ([]int64, error) {
	return s.store.SparkRoles(ctx, id)
}

// SetSparkRoles changes provisioning grants without granting Spark management.
func (s *AccessService) SetSparkRoles(ctx context.Context, id string, ids []int64) error {
	return s.store.SetSparkRoles(ctx, id, ids)
}

// CanProvision requires both the platform creation permission and a Spark grant.
func (s *AccessService) CanProvision(ctx context.Context, principal Principal, sparkID string) (bool, error) {
	if principal.Admin {
		return true, nil
	}

	if !principal.Allows("servers.create") {
		return false, nil
	}

	roles, err := s.store.SparkRoles(ctx, sparkID)

	return slices.Contains(roles, principal.RoleID), err
}
