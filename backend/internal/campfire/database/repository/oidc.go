package repository

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/limits"
	"campfire/internal/shared/utils"

	"gorm.io/gorm"
)

const oidcUsernameSuffixLength = 12

// OIDCAccount finds or provisions an identity, or links it to an authenticated account.
// Email matching requires an explicit opt-in and a verified provider claim.
func (r *AuthRepository) OIDCAccount(ctx context.Context, input OIDCAccountInput) (database.User, error) {
	var user database.User

	input.Email = utils.NormalizeEmail(input.Email)

	err := database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		if input.LinkUserID != 0 {
			var login database.UserSession

			now := time.Now()
			if err := tx.Take(&login, "hash = ? AND user_id = ? AND credential_version = ? AND expires_at > ? AND last_seen_at > ?", input.LinkSessionHash, input.LinkUserID, input.CredentialVersion, now.Unix(), now.Add(-limits.SessionIdleTimeout).Unix()).Error; err != nil {
				return ErrAuthentication
			}
		}

		var identity database.OIDCIdentity

		err := tx.Take(&identity, "issuer = ? AND subject = ?", input.Issuer, input.Subject).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if err == nil {
			if input.LinkUserID != 0 && identity.UserID != input.LinkUserID {
				return ErrAccessConflict
			}

			if err := tx.Take(&user, identity.UserID).Error; err != nil {
				return err
			}
		} else {
			if input.LinkUserID != 0 {
				if err := tx.Take(&user, input.LinkUserID).Error; err != nil {
					return err
				}

				var linked int64
				if err := tx.Model(&database.OIDCIdentity{}).Where("issuer = ? AND user_id = ?", input.Issuer, user.ID).Count(&linked).Error; err != nil {
					return err
				}

				if linked != 0 {
					return ErrAccessConflict
				}
			} else if input.Options.AutoLinkEmail && input.Email != "" {
				user, err = oidcEmailAccount(tx, input.Email, input.Options.EmailVerified)
				if err != nil {
					return err
				}
			}

			if user.ID == 0 {
				if input.Email == "" {
					return ErrEmailRequired
				}

				var count int64
				if err := tx.Model(&database.User{}).Count(&count).Error; err != nil {
					return err
				}

				if count == 0 {
					return ErrAuthentication
				}

				username, err := oidcAvailableUsername(tx, input.Username)
				if err != nil {
					return err
				}

				user = database.User{Username: username, Email: input.Email, CredentialVersion: 1, RoleID: database.UserRole}
				if err := tx.Omit("Role").Create(&user).Error; err != nil {
					return err
				}
			}

			if user.Disabled || input.LinkUserID != 0 && user.CredentialVersion != input.CredentialVersion {
				return ErrAuthentication
			}

			if err := tx.Omit("User").Create(&database.OIDCIdentity{Issuer: input.Issuer, Subject: input.Subject, UserID: user.ID}).Error; err != nil {
				return err
			}
		}

		if user.Disabled || input.LinkUserID != 0 && user.CredentialVersion != input.CredentialVersion {
			return ErrAuthentication
		}

		user.OIDCNameManaged, user.OIDCEmailManaged = true, true
		updates := map[string]any{"oidc_name_managed": user.OIDCNameManaged, "oidc_email_managed": user.OIDCEmailManaged}

		if input.Name != "" {
			user.Name = input.Name
			updates["name"] = input.Name
		}

		if input.Email != "" {
			if user.OIDCLinkEmail != nil && *user.OIDCLinkEmail != input.Email {
				user.OIDCLinkEmail = nil
				updates["oidc_link_email"] = nil
			}

			user.Email = input.Email
			updates["email"] = input.Email
		}

		if input.Picture != "" {
			user.Picture = input.Picture
			updates["picture"] = input.Picture
		}

		return tx.Model(&user).Updates(updates).Error
	})

	return user, accessError(err)
}

// OIDCAccountOptions controls linking previously unknown provider identities.
type OIDCAccountOptions struct {
	AutoLinkEmail bool
	EmailVerified bool
}

// OIDCAccountInput binds provider claims and optional local-session authorization.
type OIDCAccountInput struct {
	Issuer, Subject, Username     string
	LinkUserID, CredentialVersion int64
	LinkSessionHash               string
	Name, Email, Picture          string
	Options                       OIDCAccountOptions
}

func oidcAvailableUsername(tx *gorm.DB, preferred string) (string, error) {
	username := preferred

	for {
		var count int64
		// Deleted accounts also reserve their usernames in the unique index.
		if err := tx.Unscoped().Model(&database.User{}).Where("lower(username) = lower(?)", username).Count(&count).Error; err != nil {
			return "", err
		}

		if count == 0 {
			return username, nil
		}

		suffix := strings.ToLower(rand.Text()[:oidcUsernameSuffixLength])
		baseLength := min(len(preferred), limits.UsernameMax-len(suffix)-1)
		username = preferred[:baseLength] + "-" + suffix
	}
}

func oidcEmailAccount(tx *gorm.DB, email string, verified bool) (database.User, error) {
	var user database.User

	err := tx.Where("oidc_link_email = ?", email).Take(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return database.User{}, nil
	}

	if err != nil {
		return database.User{}, err
	}

	if !verified || user.Disabled {
		return database.User{}, ErrAuthentication
	}

	var count int64
	if err := tx.Model(&database.OIDCIdentity{}).Where("user_id = ?", user.ID).Count(&count).Error; err != nil {
		return database.User{}, err
	}

	if count != 0 {
		return database.User{}, ErrAccessConflict
	}

	return user, nil
}

// An existing subject always signs in to its original account, even if an admin
// reserves the same email elsewhere. Check in the same transaction as the reservation.
func (r *AccessRepository) checkOIDCLinkEmail(tx *gorm.DB, email string) error {
	if !r.autoLinkEmail {
		return nil
	}

	// Older profiles can contain a Unicode or mixed-case domain. Narrow by the
	// case-sensitive local-part before comparing domains using the sign-in rules.
	mailboxPrefix := email[:strings.LastIndexByte(email, '@')+1]
	linked := tx.Model(&database.OIDCIdentity{}).Select("user_id").Where("issuer = ?", r.issuer)

	var candidates []struct{ Username, Email string }
	if err := tx.Model(&database.User{}).Select("username, email").
		Where("id IN (?) AND substr(trim(email), 1, length(?)) = ?", linked, mailboxPrefix, mailboxPrefix).
		Find(&candidates).Error; err != nil {
		return err
	}

	var username string

	for _, candidate := range candidates {
		if utils.NormalizeEmail(candidate.Email) == email {
			if username != "" {
				return fmt.Errorf("%w: multiple OIDC accounts list this email. Confirm the intended username with your provider before assigning access in Users", ErrAccessConflict)
			}

			username = candidate.Username
		}
	}

	if username != "" {
		return fmt.Errorf("%w: OIDC account %q already lists this email. Confirm the account with your provider before assigning access in Users", ErrAccessConflict, username)
	}

	return nil
}

// HasOIDCIdentity reports whether an account is linked to the configured issuer.
func (r *AuthRepository) HasOIDCIdentity(ctx context.Context, userID int64, issuer string) (bool, error) {
	var count int64

	err := r.db.WithContext(ctx).Model(&database.OIDCIdentity{}).Where("user_id = ? AND issuer = ?", userID, issuer).Count(&count).Error

	return count > 0, err
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
