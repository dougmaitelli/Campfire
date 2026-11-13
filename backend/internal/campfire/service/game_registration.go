package service

import (
	"context"
	"errors"
	"maps"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
)

// registration is called with s.mu held. Definitions (including absence) are
// loaded once, not revalidated or written on each state publication.
func (s *GameIntegrationService) registration(ctx context.Context, serverID string) (*model.GameRegistration, error) {
	if registration, known := s.registrations[serverID]; known {
		return registration, nil
	}

	var registration *model.GameRegistration

	if s.Events != nil && s.Events.Store != nil {
		var err error

		registration, err = s.Events.Store.GameRegistration(ctx, serverID)
		if err != nil {
			return nil, err
		}
	}

	if s.registrations == nil {
		s.registrations = make(map[string]*model.GameRegistration)
	}

	s.registrations[serverID] = registration

	return registration, nil
}

func (s *GameIntegrationService) register(ctx context.Context, sparkID, serverID string, definition model.GameRegistration) error {
	if s.Events == nil || s.Events.Store == nil {
		return errors.New("game registration storage unavailable")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.Events.Store.RegisterGame(ctx, sparkID, serverID, definition); err != nil {
		if errors.Is(err, repository.ErrGameEventPlacement) || errors.Is(err, repository.ErrGameRegistrationConflict) {
			return &protocol.CommandError{Code: protocol.ErrorPublicationRejected, Message: "Game registration rejected; check placement and use a newer revision for changed definitions."}
		}

		return err
	}
	// Reload an independent copy so publishers cannot mutate the cached maps.
	delete(s.registrations, serverID)

	registration, err := s.registration(ctx, serverID)
	if err != nil {
		return err
	}

	if registration == nil {
		return &protocol.CommandError{Code: protocol.ErrorPublicationRejected, Message: "Game registration no longer exists."}
	}

	if record, ok := s.states[serverID]; ok && record.state != nil {
		record.state.FieldLabels = registeredStateLabels(registration, record.state.Fields)
	}

	return nil
}

// StateLabels returns definitions independently of game-value freshness. A
// registration is durable server metadata; unregistered publishers use the last
// accepted snapshot's labels. Callers receive an independent copy.
func (s *GameIntegrationService) StateLabels(ctx context.Context, serverID string) (model.GameFieldLabels, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	registration, err := s.registration(ctx, serverID)
	if err != nil {
		return nil, err
	}

	if registration != nil {
		return maps.Clone(registration.StateFields), nil
	}

	if record, ok := s.states[serverID]; ok && record.state != nil {
		return maps.Clone(record.state.FieldLabels), nil
	}

	return nil, nil
}

func registeredStateLabels(registration *model.GameRegistration, fields map[string]any) model.GameFieldLabels {
	labels := model.GameFieldLabels{}

	for key := range fields {
		if label, ok := registration.StateFields[key]; ok {
			labels[key] = label
		}
	}

	return labels
}
