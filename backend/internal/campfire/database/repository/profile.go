package repository

import (
	"context"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/limits"
	"campfire/internal/shared/utils"

	"gorm.io/gorm"
)

func profileUpdates(user database.User, name, email *string, admin bool) (map[string]any, error) {
	updates := map[string]any{}

	if name != nil && *name != user.Name {
		if user.OIDCNameManaged {
			return nil, ErrAccessDenied
		}

		updates["name"] = *name
	}

	if email != nil && *email != utils.NormalizeEmail(user.Email) {
		if user.OIDCEmailManaged {
			return nil, ErrAccessDenied
		}

		updates["oidc_link_email"] = nil
		if admin {
			updates["oidc_link_email"] = *email
		}

		updates["email"] = *email
	}

	return updates, nil
}

// UpdateProfile can update only self-service fields. It cannot change role,
// enabled state, username, or another account, even with a stale caller snapshot.
func (r *AuthRepository) UpdateProfile(ctx context.Context, session database.UserSession, name, email *string, passwordHash string, proof MFAProof, now time.Time) (database.User, error) {
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

		updates, err := profileUpdates(user, name, email, false)
		if err != nil {
			return err
		}
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
