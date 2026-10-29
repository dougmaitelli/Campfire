package service

import (
	"context"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	accessperm "campfire/internal/shared/permissions"
)

// APITokenScopes is the existing permission vocabulary plus all current access.
var APITokenScopes = append([]string{accessperm.AllAccess, accessperm.ServersCreate, accessperm.ServersManageAll, accessperm.SparksManage, accessperm.EmbersManage, accessperm.UsersManage, accessperm.RolesManage, accessperm.AuditView, accessperm.NotificationsRead, accessperm.NotificationsWrite, accessperm.NotificationsManage}, accessperm.All()...)

// CreateAPIToken returns a secret exactly once. Scopes restrict live user access
// and cannot independently grant any platform permission or server membership.
func (s *AuthService) CreateAPIToken(ctx context.Context, principal Principal, name string, days int, scopes []string) (database.APIToken, string, error) {
	name = strings.TrimSpace(name)

	if principal.APITokenID != 0 {
		return database.APIToken{}, "", repository.ErrAccessDenied
	}

	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 100 || strings.ContainsFunc(name, unicode.IsControl) || days < 1 || days > 365 || len(scopes) < 1 || len(scopes) > len(APITokenScopes) {
		return database.APIToken{}, "", repository.ErrAccessInput
	}

	scopes = slices.Clone(scopes)
	slices.Sort(scopes)

	scopes = slices.Compact(scopes)
	for _, scope := range scopes {
		if !slices.Contains(APITokenScopes, scope) {
			return database.APIToken{}, "", repository.ErrAccessInput
		}
	}

	if slices.Contains(scopes, accessperm.AllAccess) && len(scopes) != 1 {
		return database.APIToken{}, "", repository.ErrAccessInput
	}

	for _, scope := range scopes {
		if strings.HasPrefix(scope, "server.") && scope != accessperm.ServerView && !slices.Contains(scopes, accessperm.ServerView) {
			return database.APIToken{}, "", repository.ErrAccessInput
		}
	}

	scopes = accessperm.Expand(scopes)
	raw := "cf_" + authToken()
	now := s.now()

	token := database.APIToken{UserID: principal.UserID, CredentialVersion: principal.CredentialVersion, Name: name, Hash: tokenHash(raw), Prefix: raw[:11], Scopes: scopes, CreatedAt: now.Unix(), ExpiresAt: now.Add(time.Duration(days) * 24 * time.Hour).Unix()}
	if err := s.store.CreateAPIToken(ctx, &token, now); err != nil {
		return database.APIToken{}, "", err
	}

	return token, raw, nil
}

// APITokens lists only the caller's unexpired token metadata.
func (s *AuthService) APITokens(ctx context.Context, principal Principal) ([]database.APIToken, error) {
	if principal.APITokenID != 0 {
		return nil, repository.ErrAccessDenied
	}

	return s.store.APITokens(ctx, principal.UserID, principal.CredentialVersion, s.now())
}

// RevokeAPIToken invalidates one of the caller's credentials immediately.
func (s *AuthService) RevokeAPIToken(ctx context.Context, principal Principal, id int64) error {
	if principal.APITokenID != 0 {
		return repository.ErrAccessDenied
	}

	return s.store.RevokeAPIToken(ctx, principal.UserID, id)
}

// AuthenticateAPIToken derives a principal from the token and freshly loaded user.
func (s *AuthService) AuthenticateAPIToken(ctx context.Context, raw string) (Principal, error) {
	if !strings.HasPrefix(raw, "cf_") || !validAuthToken(strings.TrimPrefix(raw, "cf_")) {
		return Principal{}, ErrAuthentication
	}

	token, err := s.store.APIToken(ctx, tokenHash(raw), s.now())
	if err != nil {
		return Principal{}, err
	}

	principal := PrincipalFromSession(database.UserSession{UserID: token.UserID, User: token.User})
	principal.APITokenID, principal.TokenScopes = token.ID, token.Scopes

	return s.Access().EffectivePrincipal(ctx, principal)
}
