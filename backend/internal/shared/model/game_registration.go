package model

import (
	"errors"

	"campfire/internal/shared/limits"
)

// GameRegistration replaces publisher-owned display definitions for one server.
// Revision is monotonically increasing; an identical revision is safe to retry.
// No state values or event data are included in registration.
type GameRegistration struct {
	Revision    int64                      `json:"revision"`
	StateFields GameFieldLabels            `json:"state_fields,omitempty"`
	EventFields map[string]GameFieldLabels `json:"event_fields,omitempty"`
}

// Validate bounds definitions independently of any particular publication.
func (r GameRegistration) Validate() error {
	if r.Revision < 1 || r.Revision > 9007199254740991 || len(r.EventFields) > limits.GameFields {
		return errors.New("invalid game registration revision or event type count")
	}

	if err := r.StateFields.validateDefinitions(); err != nil {
		return err
	}

	for eventType, labels := range r.EventFields {
		if !ValidGameEventType(eventType) {
			return errors.New("invalid registered game event type")
		}

		if err := labels.validateDefinitions(); err != nil {
			return err
		}
	}

	return nil
}
