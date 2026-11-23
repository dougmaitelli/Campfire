package repository

import (
	"context"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/limits"
	"gorm.io/gorm"
)

// UpdateProfile can update only self-service fields. It cannot change role,
// enabled state, username, or another account, even with a stale caller snapshot.
func (r *AuthRepository) UpdateProfile(ctx context.Context, session database.UserSession, name, email, passwordHash string, proof MFAProof, now time.Time) (database.User, error) {
	var user database.User
	err := database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		var current database.UserSession
		if err := tx.Where("hash = ? AND user_id = ? AND expires_at > ? AND last_seen_at > ?", session.Hash, session.UserID, now.Unix(), now.Add(-limits.SessionIdleTimeout).Unix()).Take(&current).Error; err != nil {
			return ErrAuthentication
		}
		if err := tx.Preload("Role").Take(&user, session.UserID).Error; err != nil {
			return err
		}
		if user.Disabled || user.CredentialVersion != session.CredentialVersion || current.CredentialVersion != user.CredentialVersion {
			return ErrAuthentication
		}
		updates := map[string]any{"name": name, "email": email}
		if passwordHash != "" {
			if current.Provider != "" || user.PasswordHash == "" {
				return ErrAccessDenied
			}
			factor, err := readMFA(tx, user.ID)
			if err != nil {
				return err
			}
			if err := consumeMFA(tx, &factor, proof); err != nil {
				return err
			}
			updates["password_hash"], updates["credential_version"] = passwordHash, user.CredentialVersion+1
			if err := tx.Where("user_id = ? AND hash <> ?", user.ID, current.Hash).Delete(&database.UserSession{}).Error; err != nil {
				return err
			}
			if err := tx.Model(&current).Update("credential_version", user.CredentialVersion+1).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&user).Updates(updates).Error; err != nil {
			return err
		}
		return nil
	})
	return user, err
}
