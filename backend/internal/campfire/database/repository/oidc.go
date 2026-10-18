package repository

import (
	"context"
	"errors"
	"time"

	"campfire/internal/campfire/database"

	"gorm.io/gorm"
)

// OIDCAccount finds or provisions an identity, or links it to an authenticated account.
// Names and email claims never select an existing account.
func (r *AuthRepository) OIDCAccount(ctx context.Context, issuer, subject, username string, linkID, version int64, linkHash, name, picture string) (database.User, error) {
	var user database.User

	err := database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		if linkID != 0 {
			var login database.UserSession

			now := time.Now()
			if err := tx.Take(&login, "hash = ? AND user_id = ? AND credential_version = ? AND expires_at > ? AND last_seen_at > ?", linkHash, linkID, version, now.Unix(), now.Add(-30*time.Minute).Unix()).Error; err != nil {
				return ErrAuthentication
			}
		}

		var identity database.OIDCIdentity

		err := tx.Take(&identity, "issuer = ? AND subject = ?", issuer, subject).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if err == nil {
			if linkID != 0 && identity.UserID != linkID {
				return ErrAccessConflict
			}

			if err := tx.Take(&user, identity.UserID).Error; err != nil {
				return err
			}
		} else {
			if linkID != 0 {
				if err := tx.Take(&user, linkID).Error; err != nil {
					return err
				}
			} else {
				var count int64
				if err := tx.Model(&database.User{}).Count(&count).Error; err != nil {
					return err
				}

				if count == 0 {
					return ErrAuthentication
				}

				user = database.User{Username: username, CredentialVersion: 1, RoleID: database.UserRole}
				if err := tx.Omit("Role").Create(&user).Error; err != nil {
					return err
				}
			}

			if user.Disabled || linkID != 0 && user.CredentialVersion != version {
				return ErrAuthentication
			}

			if err := tx.Omit("User").Create(&database.OIDCIdentity{Issuer: issuer, Subject: subject, UserID: user.ID}).Error; err != nil {
				return err
			}
		}

		if user.Disabled || linkID != 0 && user.CredentialVersion != version {
			return ErrAuthentication
		}

		updates := map[string]any{}

		if user.Name == "" && name != "" {
			user.Name = name
			updates["name"] = name
		}

		if picture != "" {
			user.Picture = picture
			updates["picture"] = picture
		}

		if len(updates) > 0 {
			return tx.Model(&user).Updates(updates).Error
		}

		return nil
	})

	return user, accessError(err)
}

// HasOIDCAdmin prevents enabling OIDC-only login before an administrator can use it.
func (r *AuthRepository) HasOIDCAdmin(ctx context.Context, issuer string) (bool, error) {
	var count int64

	err := r.db.WithContext(ctx).Model(&database.OIDCIdentity{}).
		Joins("JOIN users ON users.id = oidc_identities.user_id").
		Where("oidc_identities.issuer = ? AND users.role_id = ? AND users.disabled = ? AND users.deleted_at IS NULL", issuer, database.AdminRole, false).
		Count(&count).Error

	return count > 0, err
}

// PruneLoginProviders permanently revokes sessions disallowed by the new policy.
func (r *AuthRepository) PruneLoginProviders(ctx context.Context, issuer string, disableLocal bool) error {
	query := r.db.WithContext(ctx).Where("provider != '' AND provider != ?", issuer)
	if disableLocal {
		query = query.Or("provider = ''")
	}

	return query.Delete(&database.UserSession{}).Error
}
