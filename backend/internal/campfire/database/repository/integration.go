package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/integrations"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Integration errors distinguish missing resources, conflicts, and disabled routes.
var (
	ErrIntegrationNotFound = errors.New("integration resource not found")
	ErrIntegrationConflict = errors.New("integration resource already exists or changed")
	ErrIntegrationDisabled = errors.New("integration connection or binding is disabled")
)

// IntegrationRepository persists provider-independent settings and outgoing messages.
// Constructing it with a transaction lets future producers enqueue atomically.
type IntegrationRepository struct{ db *gorm.DB }

// NewIntegrationRepository uses the panel database or an existing transaction.
func NewIntegrationRepository(db *gorm.DB) *IntegrationRepository {
	return &IntegrationRepository{db: db}
}

// CreateConnection inserts configuration without imposing a provider singleton.
func (r *IntegrationRepository) CreateConnection(ctx context.Context, value integrations.Connection) error {
	if err := value.Validate(); err != nil {
		return err
	}

	now := time.Now().Unix()
	row := database.Integration{ID: value.ID, Provider: value.Provider, Name: value.Name, Enabled: value.Enabled, Config: value.Config, CredentialRef: value.CredentialRef, CreatedAt: now, UpdatedAt: now}

	return integrationError(r.db.WithContext(ctx).Create(&row).Error)
}

// UpdateConnection retains provider identity and creation time.
func (r *IntegrationRepository) UpdateConnection(ctx context.Context, value integrations.Connection) error {
	if err := value.Validate(); err != nil {
		return err
	}

	result := r.db.WithContext(ctx).Model(&database.Integration{}).Where("id = ? AND provider = ?", value.ID, value.Provider).
		Updates(map[string]any{"name": value.Name, "enabled": value.Enabled, "config": string(value.Config), "credential_ref": value.CredentialRef, "updated_at": time.Now().Unix()})

	if err := integrationResult(result); err != nil {
		return err
	}

	return r.WakeCards(ctx, value.ID, "")
}

// DeleteConnection removes bindings and queued messages through existing cascades.
func (r *IntegrationRepository) DeleteConnection(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&database.Integration{}).Error
}

// GetConnection returns backend configuration; secret references are excluded from JSON.
func (r *IntegrationRepository) GetConnection(ctx context.Context, id string) (integrations.Connection, error) {
	var row database.Integration

	err := r.db.WithContext(ctx).Where("id = ?", id).Take(&row).Error

	return integrationConnection(row), integrationError(err)
}

// ListConnections returns all provider connections in deterministic creation order.
func (r *IntegrationRepository) ListConnections(ctx context.Context) ([]integrations.Connection, error) {
	var rows []database.Integration
	if err := r.db.WithContext(ctx).Order("created_at, id").Find(&rows).Error; err != nil {
		return nil, err
	}

	result := make([]integrations.Connection, 0, len(rows))
	for _, row := range rows {
		result = append(result, integrationConnection(row))
	}

	return result, nil
}

// CreateBinding enforces one binding per server, connection, and destination.
func (r *IntegrationRepository) CreateBinding(ctx context.Context, value integrations.Binding) error {
	if err := value.Validate(); err != nil {
		return err
	}

	if value.Subscriptions == nil {
		value.Subscriptions = []integrations.Subscription{}
	}

	now := time.Now().Unix()
	row := database.ServerIntegration{ID: value.ID, IntegrationID: value.IntegrationID, ServerID: value.ServerID, DestinationID: value.DestinationID, Enabled: value.Enabled, Subscriptions: value.Subscriptions, Card: value.Card, CreatedAt: now, UpdatedAt: now}

	return integrationError(r.db.WithContext(ctx).Omit(clause.Associations).Create(&row).Error)
}

// UpdateBinding preserves the current external card reference, including its old
// destination when configuration moves to a new channel. Reconciliation owns cleanup.
func (r *IntegrationRepository) UpdateBinding(ctx context.Context, value integrations.Binding) error {
	if err := value.Validate(); err != nil {
		return err
	}

	if value.Subscriptions == nil {
		value.Subscriptions = []integrations.Subscription{}
	}

	subscriptions, err := json.Marshal(value.Subscriptions)
	if err != nil {
		return err
	}

	card, err := json.Marshal(value.Card)
	if err != nil {
		return err
	}

	result := r.db.WithContext(ctx).Model(&database.ServerIntegration{}).
		Where("id = ? AND integration_id = ? AND server_id = ?", value.ID, value.IntegrationID, value.ServerID).
		Updates(map[string]any{"destination_id": value.DestinationID, "enabled": value.Enabled, "subscriptions": string(subscriptions), "card": string(card), "updated_at": time.Now().Unix()})

	if err := integrationResult(result); err != nil {
		return err
	}

	return r.WakeCards(ctx, value.IntegrationID, value.ID)
}

// GetBinding returns subscription settings and the last confirmed external card.
func (r *IntegrationRepository) GetBinding(ctx context.Context, id string) (integrations.Binding, error) {
	var row database.ServerIntegration

	err := r.db.WithContext(ctx).Where("id = ?", id).Take(&row).Error

	return integrationBinding(row), integrationError(err)
}

// ListBindings returns a server's bindings without assuming a particular provider.
func (r *IntegrationRepository) ListBindings(ctx context.Context, serverID string) ([]integrations.Binding, error) {
	var rows []database.ServerIntegration
	if err := r.db.WithContext(ctx).Where("server_id = ?", serverID).Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}

	result := make([]integrations.Binding, 0, len(rows))
	for _, row := range rows {
		result = append(result, integrationBinding(row))
	}

	return result, nil
}

// DeleteBinding removes only this destination's settings and queued messages.
func (r *IntegrationRepository) DeleteBinding(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&database.ServerIntegration{}).Error
}

// EnqueueMessage snapshots a destination and payload once for a binding/event pair.
// Replayed events return the original row without resetting its delivery metadata.
// Subscription matching belongs to the event router; this is the storage primitive.
func (r *IntegrationRepository) EnqueueMessage(ctx context.Context, bindingID, eventID string, category integrations.Category, payload json.RawMessage, now time.Time) (integrations.Message, error) {
	if err := integrations.ValidateMessage(bindingID, eventID, category, payload); err != nil {
		return integrations.Message{}, err
	}

	var row database.IntegrationMessage

	err := database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		var binding database.ServerIntegration
		if err := tx.Where("id = ?", bindingID).Take(&binding).Error; err != nil {
			return integrationError(err)
		}

		var connection database.Integration
		if err := tx.Where("id = ?", binding.IntegrationID).Take(&connection).Error; err != nil {
			return integrationError(err)
		}

		if !binding.Enabled || !connection.Enabled {
			return ErrIntegrationDisabled
		}

		row = database.IntegrationMessage{BindingID: bindingID, EventID: eventID, Category: category, DestinationID: binding.DestinationID, Payload: payload, State: model.DeliveryPending, NextAttemptAt: now.Unix(), CreatedAt: now.Unix()}
		if err := tx.Omit(clause.Associations).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "binding_id"}, {Name: "event_id"}}, DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}

		return tx.Where("binding_id = ? AND event_id = ?", bindingID, eventID).Take(&row).Error
	})

	return integrationMessage(row), err
}

func integrationConnection(row database.Integration) integrations.Connection {
	return integrations.Connection{ID: row.ID, Provider: row.Provider, Name: row.Name, Enabled: row.Enabled, Config: row.Config, CredentialRef: row.CredentialRef, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}

func integrationBinding(row database.ServerIntegration) integrations.Binding {
	return integrations.Binding{ID: row.ID, IntegrationID: row.IntegrationID, ServerID: row.ServerID, DestinationID: row.DestinationID, Enabled: row.Enabled, Subscriptions: row.Subscriptions, Card: row.Card, StatusMessage: integrations.MessageReference{DestinationID: row.MessageDestinationID, ID: row.ExternalMessageID}, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}

func integrationMessage(row database.IntegrationMessage) integrations.Message {
	return integrations.Message{ID: row.ID, BindingID: row.BindingID, EventID: row.EventID, Category: row.Category, DestinationID: row.DestinationID, Payload: row.Payload, State: row.State, Attempts: row.Attempts, NextAttemptAt: row.NextAttemptAt, DeliveredAt: row.DeliveredAt, ExternalMessageID: row.ExternalMessageID, Error: row.Error, CreatedAt: row.CreatedAt}
}

func integrationError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrIntegrationNotFound
	}

	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return ErrIntegrationConflict
	}

	return err
}

func integrationResult(result *gorm.DB) error {
	if result.Error != nil {
		return integrationError(result.Error)
	}

	if result.RowsAffected == 0 {
		return ErrIntegrationNotFound
	}

	return nil
}
