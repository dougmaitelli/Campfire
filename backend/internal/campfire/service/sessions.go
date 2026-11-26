package service

import (
	"context"
	"strings"
)

type sessionUserAgentKey struct{}

// WithSessionUserAgent attaches bounded browser metadata to a new login.
func WithSessionUserAgent(ctx context.Context, agent string) context.Context {
	agent = strings.ToValidUTF8(agent, "")

	runes := []rune(agent)
	if len(runes) > 512 {
		agent = string(runes[:512])
	}

	return context.WithValue(ctx, sessionUserAgentKey{}, agent)
}

func sessionUserAgent(ctx context.Context) string {
	agent, _ := ctx.Value(sessionUserAgentKey{}).(string)
	return agent
}

// LoginSession exposes session metadata without credentials or stored token hashes.
type LoginSession struct {
	ID         string `json:"id"`
	UserAgent  string `json:"user_agent"`
	CreatedAt  int64  `json:"created_at"`
	LastSeenAt int64  `json:"last_seen_at"`
	ExpiresAt  int64  `json:"expires_at"`
	Current    bool   `json:"current"`
}

// Sessions lists the authenticated browser owner's usable sessions.
func (s *AuthService) Sessions(ctx context.Context, token string) ([]LoginSession, error) {
	current, _, err := s.Session(ctx, token)
	if err != nil {
		return nil, err
	}

	sessions, err := s.store.Sessions(ctx, current, s.now())
	if err != nil {
		return nil, err
	}

	local, err := s.LocalLoginEnabled(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]LoginSession, 0, len(sessions))
	for _, session := range sessions {
		if session.Provider == "" && !local || session.Provider != "" && session.Provider != s.loginProvider {
			continue
		}

		result = append(result, LoginSession{ID: tokenHash(session.Hash), UserAgent: session.UserAgent, CreatedAt: session.CreatedAt, LastSeenAt: session.LastSeenAt, ExpiresAt: session.ExpiresAt, Current: session.Hash == current.Hash})
	}

	return result, nil
}

// RevokeSession resolves a public ID only within the current account's sessions.
// Unknown IDs are harmless, including IDs belonging to another account.
func (s *AuthService) RevokeSession(ctx context.Context, token, id string) (bool, error) {
	current, _, err := s.Session(ctx, token)
	if err != nil {
		return false, err
	}

	sessions, err := s.store.Sessions(ctx, current, s.now())
	if err != nil {
		return false, err
	}

	for _, session := range sessions {
		if tokenHash(session.Hash) == id {
			return session.Hash == current.Hash, s.store.RevokeOwnedSession(ctx, current.UserID, session.Hash)
		}
	}

	return false, nil
}

// RevokeOtherSessions signs out the account's other browsers.
func (s *AuthService) RevokeOtherSessions(ctx context.Context, token string) error {
	current, _, err := s.Session(ctx, token)
	if err != nil {
		return err
	}

	return s.store.RevokeOtherSessions(ctx, current)
}
