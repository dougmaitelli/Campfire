package repository

import (
	"context"
	"errors"
	"time"

	"campfire/internal/database"

	"gorm.io/gorm"
)

// Authentication errors deliberately reveal no credential details.
var (
	ErrAuthentication = errors.New("authentication required")
	ErrAdminExists    = errors.New("administrator already configured")
)

// AuthRepository serializes setup, session validation and password replacement.
type AuthRepository struct{ db *gorm.DB }

// NewAuthRepository constructs persistent administrator storage.
func NewAuthRepository(db *gorm.DB) *AuthRepository { return &AuthRepository{db: db} }

// Credential returns the singleton administrator, or nil before setup.
func (r *AuthRepository) Credential(ctx context.Context) (*database.AdminCredential, error) {
	var admin database.AdminCredential
	if err := r.db.WithContext(ctx).Take(&admin, 1).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}

		return nil, err
	}

	return &admin, nil
}

// Setup atomically creates exactly one administrator and their initial session.
func (r *AuthRepository) Setup(ctx context.Context, admin database.AdminCredential, session database.AdminSession) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&database.AdminCredential{}).Count(&count).Error; err != nil {
			return err
		}

		if count != 0 {
			return ErrAdminExists
		}

		if err := tx.Create(&admin).Error; err != nil {
			return err
		}

		return tx.Create(&session).Error
	})
}

// CreateSession checks the credential version again to reject logins racing a reset.
func (r *AuthRepository) CreateSession(ctx context.Context, session database.AdminSession, previous string, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var admin database.AdminCredential
		if err := tx.Take(&admin, 1).Error; err != nil {
			return err
		}

		if admin.Version != session.CredentialVersion {
			return ErrAuthentication
		}

		if err := tx.Where("hash = ? OR expires_at <= ? OR last_seen_at <= ?", previous, now.Unix(), now.Add(-30*time.Minute).Unix()).Delete(&database.AdminSession{}).Error; err != nil {
			return err
		}

		var oldest []string
		if err := tx.Model(&database.AdminSession{}).Order("last_seen_at DESC, hash").Offset(19).Pluck("hash", &oldest).Error; err != nil {
			return err
		}

		if len(oldest) != 0 {
			if err := tx.Where("hash IN ?", oldest).Delete(&database.AdminSession{}).Error; err != nil {
				return err
			}
		}

		return tx.Create(&session).Error
	})
}

// Session verifies expiry and credential generation before refreshing activity.
func (r *AuthRepository) Session(ctx context.Context, hash string, now time.Time) (database.AdminSession, string, error) {
	var session database.AdminSession

	var username string

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("hash = ? AND expires_at > ? AND last_seen_at > ?", hash, now.Unix(), now.Add(-30*time.Minute).Unix()).Take(&session).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAuthentication
			}

			return err
		}

		var admin database.AdminCredential
		if err := tx.Take(&admin, 1).Error; err != nil {
			return err
		}

		if admin.Version != session.CredentialVersion {
			return ErrAuthentication
		}

		username = admin.Username

		if now.Unix()-session.LastSeenAt >= 60 {
			return tx.Model(&session).Update("last_seen_at", now.Unix()).Error
		}

		return nil
	})

	return session, username, err
}

// Revoke removes a session immediately; unknown identities are harmless.
func (r *AuthRepository) Revoke(ctx context.Context, hash string) error {
	return r.db.WithContext(ctx).Where("hash = ?", hash).Delete(&database.AdminSession{}).Error
}

// ResetPassword changes the password and revokes every session in one transaction.
func (r *AuthRepository) ResetPassword(ctx context.Context, encoded string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&database.AdminCredential{}).Where("id = 1").Updates(map[string]any{"password_hash": encoded, "version": gorm.Expr("version + 1")})
		if result.Error != nil {
			return result.Error
		}

		if result.RowsAffected != 1 {
			return ErrAuthentication
		}

		return tx.Where("1 = 1").Delete(&database.AdminSession{}).Error
	})
}
