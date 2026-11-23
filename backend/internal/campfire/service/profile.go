package service

import (
	"context"
	"strings"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
)

// UpdateProfile updates the authenticated account's display details. A password
// change requires the current password and any enrolled second factor.
func (s *AuthService) UpdateProfile(ctx context.Context, token, name, email, password, currentPassword, code string) (database.User, error) {
	name, email = strings.TrimSpace(name), strings.TrimSpace(email)
	if !validDisplayName(name) || !validEmail(email) {
		return database.User{}, repository.ErrAccessInput
	}
	session, _, err := s.Session(ctx, token)
	if err != nil {
		return database.User{}, err
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
		var factor database.UserMFA
		session, factor, err = s.mfaAccount(ctx, token, currentPassword)
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
	return s.store.UpdateProfile(ctx, session, name, email, encoded, proof, s.now())
}
