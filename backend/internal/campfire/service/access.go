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
	accessperm "campfire/internal/shared/permissions"
)

// Principal is derived from persisted credentials, never from client-supplied roles.
type Principal struct {
	APITokenID        int64    `json:"api_token_id,omitempty"`
	TokenScopes       []string `json:"-"`
	ScopeLimit        []string `json:"-"`
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
	return Principal{UserID: session.UserID, Username: session.User.Username, Name: session.User.Name, Picture: session.User.Picture, RoleID: session.User.RoleID, Role: session.User.Role.Name, Admin: session.User.RoleID == database.AdminRole, Permissions: roleActions(session.User.Role), CredentialVersion: session.User.CredentialVersion}
}

// Allows checks a platform action against the user's permissions and token scopes.
func (p Principal) Allows(action string) bool {
	return p.tokenAllows(action) && (p.Admin || slices.Contains(p.Permissions, action))
}

func (p Principal) tokenAllows(action string) bool {
	if p.ScopeLimit != nil && !slices.Contains(p.ScopeLimit, accessperm.AllAccess) && !slices.Contains(p.ScopeLimit, action) {
		return false
	}

	return p.APITokenID == 0 || slices.Contains(p.TokenScopes, accessperm.AllAccess) || slices.Contains(p.TokenScopes, action)
}

func (p Principal) restrictServer(actions []string) []string {
	actions = accessperm.Expand(actions)
	if p.APITokenID == 0 && p.ScopeLimit == nil {
		return actions
	}

	return accessperm.Restrict(slices.DeleteFunc(slices.Clone(actions), func(action string) bool { return !p.tokenAllows(action) }))
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

// CanManageServer requires actual ownership; a custom role cannot delegate access.
func (s *AccessService) CanManageServer(ctx context.Context, principal Principal, id string) (bool, error) {
	if principal.APITokenID != 0 && !slices.Contains(principal.TokenScopes, accessperm.AllAccess) {
		return false, nil
	}

	owner, err := s.store.Owner(ctx, id)

	return principal.Admin || owner != nil && owner.ID == principal.UserID, err
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

	allowed := []string{accessperm.ServersCreate}
	if scope == "server" {
		allowed = accessperm.All()
	}

	role := database.Role{ID: id, Name: name, Scope: scope}
	if scope == "server" {
		for _, action := range permissions {
			if !slices.Contains(allowed, action) {
				return RoleView{}, repository.ErrAccessInput
			}
		}

		permissions = accessperm.Expand(permissions)
	}

	slices.Sort(permissions)

	for _, permission := range slices.Compact(permissions) {
		if !slices.Contains(allowed, permission) {
			return RoleView{}, repository.ErrAccessInput
		}

		role.Permissions = append(role.Permissions, database.RolePermission{Permission: permission})
	}

	if scope == "server" && !slices.Contains(permissions, accessperm.ServerView) {
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

	user := database.User{Username: username, Name: name, Email: email, PasswordHash: encoded, CredentialVersion: 1, RoleID: roleID, Disabled: disabled}
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
func (s *AccessService) ServerAccess(ctx context.Context, id string) (*database.User, []database.ServerAccessGrant, error) {
	owner, err := s.store.Owner(ctx, id)
	if err != nil {
		return nil, nil, err
	}

	members, err := s.store.Members(ctx, id)

	return owner, members, err
}

// Share lets owners and administrators assign a server role to a collaborator.
func (s *AccessService) Share(ctx context.Context, principal Principal, id, username string, roleID int64) error {
	allowed, err := s.CanManageServer(ctx, principal, id)
	if err != nil {
		return err
	}

	if !allowed {
		return repository.ErrAccessDenied
	}

	user, err := s.auth.store.FindCredential(ctx, strings.ToLower(strings.TrimSpace(username)))
	if err != nil {
		return err
	}

	if user == nil || user.Disabled {
		return repository.ErrAccessInput
	}

	return s.store.SetMember(ctx, database.ServerAccessGrant{ServerID: id, UserID: &user.ID, AccessRoleID: roleID})
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

// CanProvision grants creation only on the Spark explicitly assigned to this subject.
func (s *AccessService) CanProvision(ctx context.Context, principal Principal, sparkID string) (bool, error) {
	if principal.Admin {
		return true, nil
	}

	if !principal.tokenAllows(accessperm.ServersCreate) {
		return false, nil
	}

	return s.store.HasSparkGrant(ctx, principal.UserID, principal.RoleID, sparkID)
}

// EffectivePrincipal exposes scoped creation eligibility without granting Spark management.
func (s *AccessService) EffectivePrincipal(ctx context.Context, principal Principal) (Principal, error) {
	if principal.Admin || slices.Contains(principal.Permissions, accessperm.ServersCreate) {
		return principal, nil
	}

	allowed, err := s.store.HasSparkGrant(ctx, principal.UserID, principal.RoleID, "")
	if err == nil && allowed {
		principal.Permissions = append(slices.Clone(principal.Permissions), accessperm.ServersCreate)
	}

	return principal, err
}

// AccessGrantInput targets exactly one user or account role and specifies the access level.
type AccessGrantInput struct {
	Username     string `json:"username"`
	RoleID       *int64 `json:"role_id"`
	AccessRoleID int64  `json:"access_role_id"`
}

func (s *AccessService) grantSubject(ctx context.Context, input AccessGrantInput) (*int64, error) {
	if (strings.TrimSpace(input.Username) == "") == (input.RoleID == nil) {
		return nil, repository.ErrAccessInput
	}

	if input.RoleID != nil {
		return nil, nil
	}

	user, err := s.auth.store.FindCredential(ctx, strings.ToLower(strings.TrimSpace(input.Username)))
	if err != nil {
		return nil, err
	}

	if user == nil || user.Disabled {
		return nil, repository.ErrAccessInput
	}

	return &user.ID, nil
}

// GrantServer validates and saves a server access grant.
func (s *AccessService) GrantServer(ctx context.Context, principal Principal, id string, input AccessGrantInput) error {
	allowed, err := s.CanManageServer(ctx, principal, id)
	if err != nil {
		return err
	}

	if !allowed {
		return repository.ErrAccessDenied
	}

	userID, err := s.grantSubject(ctx, input)
	if err != nil {
		return err
	}

	return s.store.SetMember(ctx, database.ServerAccessGrant{ServerID: id, UserID: userID, RoleID: input.RoleID, AccessRoleID: input.AccessRoleID})
}

// GrantSpark validates and saves a Spark creation grant.
func (s *AccessService) GrantSpark(ctx context.Context, id string, input AccessGrantInput) error {
	if input.AccessRoleID != 0 && input.AccessRoleID != database.CreatorRole {
		return repository.ErrAccessInput
	}

	userID, err := s.grantSubject(ctx, input)
	if err != nil {
		return err
	}

	return s.store.SetSparkGrant(ctx, database.SparkAccessGrant{SparkID: id, UserID: userID, RoleID: input.RoleID, AccessRoleID: database.CreatorRole})
}

// SparkGrants lists explicit Spark provisioning grants.
func (s *AccessService) SparkGrants(ctx context.Context, id string) ([]database.SparkAccessGrant, error) {
	return s.store.SparkGrants(ctx, id)
}

// RemoveServerGrant removes one server access grant.
func (s *AccessService) RemoveServerGrant(ctx context.Context, id string, grantID int64) error {
	return s.store.RemoveServerGrant(ctx, id, grantID)
}

// RemoveSparkGrant revokes one Spark provisioning grant.
func (s *AccessService) RemoveSparkGrant(ctx context.Context, id string, grantID int64) error {
	return s.store.RemoveSparkGrant(ctx, id, grantID)
}
