package repository

import (
	"context"
	"errors"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/events"

	"gorm.io/gorm"
)

// ErrUnsupportedEvent rejects publications with no registered application policy.
var ErrUnsupportedEvent = errors.New("unsupported application event")

// EventPublication contains identities created by publication handlers. Inside a
// source transaction, they become durable only when that outer transaction commits.
type EventPublication struct {
	NotificationID int64
}

// EventPublisher runs the explicit application handlers in one transaction.
// Call it with a source transaction to commit its record and all follow-ups
// together. Handlers only write local records and durable outgoing work; they
// never call providers. Source repositories retain their idempotency guards.
type EventPublisher struct {
	db            *gorm.DB
	queueExternal bool
}

// NewEventPublisher shares the caller's database/transaction and Apprise setting.
// Handler registration is fixed here, rather than in a mutable global event bus.
func NewEventPublisher(db *gorm.DB, queueExternal bool) *EventPublisher {
	return &EventPublisher{db: db, queueExternal: queueExternal}
}

func eventPublisher(db *gorm.DB, publishers []*EventPublisher) *EventPublisher {
	if len(publishers) > 0 && publishers[0] != nil {
		return publishers[0].inTransaction(db)
	}

	return NewEventPublisher(db, false)
}

func (p *EventPublisher) inTransaction(tx *gorm.DB) *EventPublisher {
	return &EventPublisher{db: tx, queueExternal: p.queueExternal}
}

// Publish dispatches one application fact. Any handler failure rolls back every
// follow-up, including work queued by earlier handlers, and returns no identities.
func (p *EventPublisher) Publish(ctx context.Context, event events.Event, at time.Time) (EventPublication, error) {
	publication := EventPublication{}

	err := database.Transaction(p.db.WithContext(ctx), func(tx *gorm.DB) error {
		handlers := []func(context.Context, *gorm.DB, events.Event, time.Time, *EventPublication) (bool, error){
			p.monitoringAlert,
			p.auditEvent,
			p.panelNotification,
			p.integrationMessages,
		}
		handled := false

		for _, handler := range handlers {
			accepted, err := handler(ctx, tx, event, at, &publication)
			if err != nil {
				return err
			}

			handled = handled || accepted
		}

		if !handled {
			return ErrUnsupportedEvent
		}

		return nil
	})
	if err != nil {
		return EventPublication{}, err
	}

	return publication, nil
}
