package repository

import (
	"context"
	"errors"
	"slices"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/limits"
	"gorm.io/gorm"
)

// ErrMFARequired reports a missing second factor only after password verification.
var ErrMFARequired = errors.New("enter an authenticator code or recovery code")

// MFAProof is produced by the service after cryptographic verification. A secret
// reference binds the proof to the precise enrollment that was verified.
type MFAProof struct {
	SecretRef    string
	Counter      int64
	RecoveryHash string
}

func readMFA(tx *gorm.DB, userID int64) (database.UserMFA, error) {
	factor := database.UserMFA{UserID: userID}
	err := tx.Where("user_id = ?", userID).Take(&factor).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = nil
	}
	if factor.SecretRef != "" && len(factor.Devices) == 0 {
		factor.Devices = []database.MFADevice{{ID: "legacy", Name: "Authenticator", SecretRef: factor.SecretRef, LastCounter: factor.LastCounter}}
	}
	return factor, err
}

// MFA reads private factor state for service-side verification.
func (r *AuthRepository) MFA(ctx context.Context, userID int64) (database.UserMFA, error) {
	return readMFA(r.db.WithContext(ctx), userID)
}

func consumeMFA(tx *gorm.DB, factor *database.UserMFA, proof MFAProof) error {
	if factor.SecretRef == "" {
		return nil
	}
	if proof.SecretRef == "" {
		return ErrMFARequired
	}
	if proof.RecoveryHash != "" {
		if proof.SecretRef != factor.SecretRef {
			return ErrAuthentication
		}
		index := slices.Index(factor.RecoveryHashes, proof.RecoveryHash)
		if index < 0 {
			return ErrAuthentication
		}
		factor.RecoveryHashes = slices.Delete(factor.RecoveryHashes, index, index+1)
	} else {
		index := slices.IndexFunc(factor.Devices, func(device database.MFADevice) bool { return device.SecretRef == proof.SecretRef })
		if index < 0 || proof.Counter <= factor.Devices[index].LastCounter {
			return ErrAuthentication
		}
		factor.Devices[index].LastCounter = proof.Counter
		if index == 0 {
			factor.LastCounter = proof.Counter
		}
	}
	return tx.Omit("User").Save(factor).Error
}

// ChangeMFA serializes session revalidation, proof consumption and factor changes.
// This prevents concurrent replay and prevents stale sessions completing enrollment.
func (r *AuthRepository) ChangeMFA(ctx context.Context, session database.UserSession, now time.Time, change func(*database.UserMFA) error, proof *MFAProof, revokeOthers bool) error {
	return database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		var current database.UserSession
		if err := tx.Where("hash = ? AND expires_at > ? AND last_seen_at > ?", session.Hash, now.Unix(), now.Add(-limits.SessionIdleTimeout).Unix()).Take(&current).Error; err != nil {
			return ErrAuthentication
		}
		var user database.User
		if err := tx.Take(&user, session.UserID).Error; err != nil {
			return err
		}
		if user.Disabled || user.CredentialVersion != session.CredentialVersion || current.CredentialVersion != session.CredentialVersion {
			return ErrAuthentication
		}
		factor, err := readMFA(tx, session.UserID)
		if err != nil {
			return err
		}
		if proof != nil {
			if err := consumeMFA(tx, &factor, *proof); err != nil {
				return err
			}
		}
		if err := change(&factor); err != nil {
			return err
		}
		if err := tx.Omit("User").Save(&factor).Error; err != nil {
			return err
		}
		if revokeOthers {
			if err := tx.Where("user_id = ? AND hash <> ?", session.UserID, session.Hash).Delete(&database.UserSession{}).Error; err != nil {
				return err
			}

		}
		return nil
	})
}

// ResetMFA is the host recovery operation.
func (r *AuthRepository) ResetMFA(ctx context.Context, username string) error {
	return database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		var user database.User
		if err := tx.Where("lower(username) = lower(?)", username).Take(&user).Error; err != nil {
			return accessError(err)
		}
		return resetMFA(tx, user)
	})
}

func resetMFA(tx *gorm.DB, user database.User) error {
	if user.PasswordHash == "" {
		return ErrAccessInput
	}
	factor := database.UserMFA{UserID: user.ID, RecoveryHashes: []string{}}
	if err := tx.Omit("User").Save(&factor).Error; err != nil {
		return err
	}
	if err := tx.Where("user_id = ?", user.ID).Delete(&database.UserSession{}).Error; err != nil {
		return err
	}
	return nil
}

// AdminResetMFA rechecks the administrator session and role in the same
// transaction as revocation, including when another request changes their role.
func (r *AuthRepository) AdminResetMFA(ctx context.Context, admin database.UserSession, userID int64, now time.Time) error {
	return database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		var session database.UserSession
		if err := tx.Where("hash = ? AND expires_at > ? AND last_seen_at > ?", admin.Hash, now.Unix(), now.Add(-limits.SessionIdleTimeout).Unix()).Take(&session).Error; err != nil {
			return ErrAuthentication
		}
		var actor database.User
		if err := tx.Take(&actor, session.UserID).Error; err != nil {
			return ErrAuthentication
		}
		if actor.Disabled || actor.RoleID != database.AdminRole || actor.CredentialVersion != admin.CredentialVersion || session.CredentialVersion != actor.CredentialVersion || userID == actor.ID {
			return ErrAccessDenied
		}
		var user database.User
		if err := tx.Take(&user, userID).Error; err != nil {
			return accessError(err)
		}
		target, err := readMFA(tx, userID)
		if err != nil {
			return err
		}
		if target.SecretRef == "" {
			return ErrAccessInput
		}
		return resetMFA(tx, user)
	})
}
