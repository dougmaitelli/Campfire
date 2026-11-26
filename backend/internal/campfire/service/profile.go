package service

import (
	"context"
	"strings"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/utils"
)

// UpdateProfile updates the authenticated account's display details. A password
// change requires the current password and any enrolled second factor.
func (s *AuthService) UpdateProfile(ctx context.Context, token string, name, email *string, password, currentPassword, code string) (database.User, error) {
	if name != nil {
		trimmed := strings.TrimSpace(*name)
		if !validDisplayName(trimmed) {
			return database.User{}, repository.ErrAccessInput
		}

		name = &trimmed
	}

	session, _, err := s.Session(ctx, token)
	if err != nil {
		return database.User{}, err
	}

	var changedEmail *string

	if email != nil {
		address := utils.NormalizeEmail(*email)
		if !validEmail(address) {
			return database.User{}, repository.ErrEmailRequired
		}

		if address != utils.NormalizeEmail(session.User.Email) {
			if session.User.OIDCEmailManaged {
				return database.User{}, repository.ErrAccessDenied
			}

			changedEmail = &address
		}
	}

	var proof repository.MFAProof

	encoded := ""

	if password != "" {
		if session.Provider != "" {
			return database.User{}, repository.ErrAccessDenied
		}

		if !validAccountInput("admin", password) {
			return database.User{}, ErrAuthInput
		}

		factor, err := s.mfaAfterPassword(ctx, session, currentPassword)
		if err != nil {
			return database.User{}, err
		}

		proof, err = s.factorProof(factor, code)
		if err != nil {
			return database.User{}, err
		}

		release, err := s.passwordWork(ctx)
		if err != nil {
			return database.User{}, err
		}
		defer release()

		encoded, err = passwordHash(password)
		if err != nil {
			return database.User{}, err
		}
	}

	return s.store.UpdateProfile(ctx, session, name, changedEmail, encoded, proof, s.now())
}
