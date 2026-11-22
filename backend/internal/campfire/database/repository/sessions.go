package repository

import (
	"context"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/limits"
)

// Sessions returns only unexpired sessions for the account's current credentials.
func (r *AuthRepository) Sessions(ctx context.Context, current database.UserSession, now time.Time) ([]database.UserSession, error) {
	sessions := make([]database.UserSession, 0)
	err := r.db.WithContext(ctx).Where("user_id = ? AND credential_version = ? AND expires_at > ? AND last_seen_at > ?", current.UserID, current.CredentialVersion, now.Unix(), now.Add(-limits.SessionIdleTimeout).Unix()).Order("last_seen_at DESC, hash").Find(&sessions).Error
	return sessions, err
}

// RevokeOwnedSession scopes deletion to the authenticated account.
func (r *AuthRepository) RevokeOwnedSession(ctx context.Context, userID int64, hash string) error {
	return r.db.WithContext(ctx).Where("user_id = ? AND hash = ?", userID, hash).Delete(&database.UserSession{}).Error
}

// RevokeOtherSessions preserves this browser while removing the account's other logins.
func (r *AuthRepository) RevokeOtherSessions(ctx context.Context, current database.UserSession) error {
	return r.db.WithContext(ctx).Where("user_id = ? AND hash <> ?", current.UserID, current.Hash).Delete(&database.UserSession{}).Error
}
