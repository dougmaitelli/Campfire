package repository

import (
	"context"
	"errors"
	"time"

	"campfire/internal/campfire/database"

	"gorm.io/gorm"
)

// CreateAPIToken enforces account generation and a maximum of 50 active tokens.
func (r *AuthRepository) CreateAPIToken(ctx context.Context, token *database.APIToken, now time.Time) error {
	return database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		var user database.User
		if err := tx.Take(&user, token.UserID).Error; err != nil {
			return ErrAuthentication
		}

		if user.Disabled || user.Version != token.CredentialVersion {
			return ErrAuthentication
		}

		if err := tx.Where("user_id = ? AND (expires_at <= ? OR credential_version <> ?)", user.ID, now.Unix(), user.Version).Delete(&database.APIToken{}).Error; err != nil {
			return err
		}

		var count int64
		if err := tx.Model(&database.APIToken{}).Where("user_id = ?", user.ID).Count(&count).Error; err != nil {
			return err
		}

		if count >= 50 {
			return ErrAccessConflict
		}

		return tx.Omit("User").Create(token).Error
	})
}

// APITokens lists active metadata for the current account generation.
func (r *AuthRepository) APITokens(ctx context.Context, userID, version int64, now time.Time) ([]database.APIToken, error) {
	items := make([]database.APIToken, 0)
	err := r.db.WithContext(ctx).Where("user_id = ? AND credential_version = ? AND expires_at > ?", userID, version, now.Unix()).Order("created_at DESC, id DESC").Find(&items).Error

	return items, err
}

// APIToken validates revocation, expiry and live account/role state on each use.
func (r *AuthRepository) APIToken(ctx context.Context, hash string, now time.Time) (database.APIToken, error) {
	var token database.APIToken

	err := database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		if err := tx.Where("hash = ? AND expires_at > ?", hash, now.Unix()).Take(&token).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAuthentication
			}

			return err
		}

		if err := tx.Preload("Role.Permissions").Take(&token.User, token.UserID).Error; err != nil {
			return ErrAuthentication
		}

		if token.User.Disabled || token.User.Version != token.CredentialVersion {
			return ErrAuthentication
		}

		if now.Unix()-token.LastUsedAt >= 60 {
			return tx.Model(&token).Omit("User").Update("last_used_at", now.Unix()).Error
		}

		return nil
	})

	return token, err
}

// RevokeAPIToken only removes a token belonging to its authenticated owner.
func (r *AuthRepository) RevokeAPIToken(ctx context.Context, userID, id int64) error {
	result := r.db.WithContext(ctx).Where("user_id = ? AND id = ?", userID, id).Delete(&database.APIToken{})
	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected != 1 {
		return ErrAccessNotFound
	}

	return nil
}
