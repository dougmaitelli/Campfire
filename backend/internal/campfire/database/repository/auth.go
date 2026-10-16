package repository

import (
	"context"
	"errors"
	"time"

	"campfire/internal/campfire/database"

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

// Credential returns the first account, or nil before initial setup.
func (r *AuthRepository) Credential(ctx context.Context) (*database.User, error) {
	var admin database.User
	if err := r.db.WithContext(ctx).Order("id").First(&admin).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}

		return nil, err
	}

	return &admin, nil
}

// FindCredential looks up one normalized username without exposing lookup failures.
func (r *AuthRepository) FindCredential(ctx context.Context, username string) (*database.User, error) {
	var user database.User
	if err := r.db.WithContext(ctx).Where("lower(username) = lower(?)", username).Take(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}

		return nil, err
	}

	return &user, nil
}

// Access constructs account and permission storage on the same database.
func (r *AuthRepository) Access() *AccessRepository { return NewAccessRepository(r.db) }

// Setup atomically creates exactly one administrator and their initial session.
func (r *AuthRepository) Setup(ctx context.Context, admin database.User, session database.UserSession) error {
	return database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&database.User{}).Count(&count).Error; err != nil {
			return err
		}

		if count != 0 {
			return ErrAdminExists
		}

		admin.RoleID = database.AdminRole
		if err := tx.Omit("Role").Create(&admin).Error; err != nil {
			return err
		}

		session.UserID = admin.ID

		return tx.Omit("User").Create(&session).Error
	})
}

// CreateSession checks the credential version again to reject logins racing a reset.
func (r *AuthRepository) CreateSession(ctx context.Context, session database.UserSession, previous string, now time.Time) error {
	return database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		var admin database.User
		if err := tx.Take(&admin, session.UserID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAuthentication
			}

			return err
		}

		if admin.Disabled || admin.Version != session.CredentialVersion {
			return ErrAuthentication
		}

		if err := tx.Where("hash = ? OR expires_at <= ? OR last_seen_at <= ?", previous, now.Unix(), now.Add(-30*time.Minute).Unix()).Delete(&database.UserSession{}).Error; err != nil {
			return err
		}

		var oldest []string
		if err := tx.Model(&database.UserSession{}).Where("user_id = ?", session.UserID).Order("last_seen_at DESC, hash").Offset(19).Pluck("hash", &oldest).Error; err != nil {
			return err
		}

		if len(oldest) != 0 {
			if err := tx.Where("hash IN ?", oldest).Delete(&database.UserSession{}).Error; err != nil {
				return err
			}
		}

		return tx.Omit("User").Create(&session).Error
	})
}

// Session verifies expiry and credential generation before refreshing activity.
func (r *AuthRepository) Session(ctx context.Context, hash string, now time.Time) (database.UserSession, string, error) {
	var session database.UserSession

	var username string

	err := database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		if err := tx.Where("hash = ? AND expires_at > ? AND last_seen_at > ?", hash, now.Unix(), now.Add(-30*time.Minute).Unix()).Take(&session).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAuthentication
			}

			return err
		}

		var admin database.User
		if err := tx.Preload("Role.Permissions").Take(&admin, session.UserID).Error; err != nil {
			return err
		}

		if admin.Disabled || admin.Version != session.CredentialVersion {
			return ErrAuthentication
		}

		username = admin.Username
		session.User = admin

		if now.Unix()-session.LastSeenAt >= 60 {
			return tx.Model(&session).Omit("User").Update("last_seen_at", now.Unix()).Error
		}

		return nil
	})

	return session, username, err
}

// Revoke removes a session immediately; unknown identities are harmless.
func (r *AuthRepository) Revoke(ctx context.Context, hash string) error {
	return r.db.WithContext(ctx).Where("hash = ?", hash).Delete(&database.UserSession{}).Error
}

// ResetPassword recovers the first administrator and revokes that account's sessions.
func (r *AuthRepository) ResetPassword(ctx context.Context, encoded string) error {
	return r.ResetPasswordFor(ctx, "", encoded)
}

// ResetPasswordFor recovers one administrator without revoking other accounts' sessions.
func (r *AuthRepository) ResetPasswordFor(ctx context.Context, username, encoded string) error {
	return database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		var admin database.User

		query := tx.Where("role_id = ?", database.AdminRole)
		if username != "" {
			query = query.Where("lower(username) = lower(?)", username)
		}

		if err := query.Order("id").First(&admin).Error; err != nil {
			return ErrAuthentication
		}

		result := tx.Model(&database.User{}).Where("id = ?", admin.ID).Updates(map[string]any{"password_hash": encoded, "disabled": false, "version": gorm.Expr("version + 1")})
		if result.Error != nil {
			return result.Error
		}

		if result.RowsAffected != 1 {
			return ErrAuthentication
		}

		return tx.Where("user_id = ?", admin.ID).Delete(&database.UserSession{}).Error
	})
}
