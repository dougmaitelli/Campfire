package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/integrations"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// IntegrationClaim fences completion with both attempt number and lease expiry.
type IntegrationClaim struct {
	integrations.Message
	Connection integrations.Connection
	ServerName string
	Reconcile  bool
}

// destination can be a bound value or an internal column reference. Both idle
// checks and transactional claims gate the destination actually being mutated.
func integrationDeliveryGate(destination any, now time.Time) clause.Expr {
	busy := []model.DeliveryState{integrations.MessageClaimed, integrations.MessageSending, integrations.MessageReconciling, integrations.MessageRateLimited, integrations.CardClaimed, integrations.CardSending}
	return clause.Expr{SQL: "NOT EXISTS (SELECT 1 FROM integration_messages busy JOIN server_integrations bb ON bb.id = busy.binding_id WHERE bb.integration_id = b.integration_id AND (busy.state = ? OR (busy.next_attempt_at > ? AND busy.state IN ?) OR (busy.destination_id = ? AND (busy.state = ? OR (busy.next_attempt_at > ? AND busy.state IN ?)))))", Vars: []any{integrations.MessageCredentialsBlocked, now.Unix(), []model.DeliveryState{integrations.MessageGlobalLimited, integrations.MessageGlobalUncertain}, destination, integrations.MessageDestinationBlocked, now.Unix(), busy}}
}

// ClaimMessage serializes selection across panel processes without holding a
// database transaction open during HTTP. One destination is processed at a time.
func (r *IntegrationRepository) ClaimMessage(ctx context.Context, now time.Time) (*IntegrationClaim, error) {
	var live []string

	ready := []model.DeliveryState{model.DeliveryPending, integrations.MessageRateLimited, integrations.MessageGlobalLimited, integrations.MessageUncertain, integrations.MessageGlobalUncertain, integrations.MessageClaimed, integrations.MessageSending, integrations.MessageReconciling}

	if err := r.db.WithContext(ctx).Table("server_integrations AS b").Joins("JOIN integrations c ON c.id = b.integration_id").
		Where("b.enabled = ? AND c.enabled = ?", true, true).
		Where("EXISTS (SELECT 1 FROM integration_messages ready WHERE ready.binding_id = b.id AND ready.category <> ? AND ready.state IN ? AND ready.next_attempt_at > 0 AND ready.next_attempt_at <= ?)", integrations.CardCategory, ready, now.Unix()).
		Where(integrationDeliveryGate(clause.Column{Table: "b", Name: "destination_id"}, now)).Pluck("b.id", &live).Error; err != nil {
		return nil, err
	}

	if len(live) == 0 {
		return nil, nil
	}

	var claim *IntegrationClaim

	err := database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		for _, recovery := range []struct {
			states []model.DeliveryState
			next   model.DeliveryState
		}{
			{[]model.DeliveryState{integrations.MessageClaimed}, model.DeliveryPending},
			{[]model.DeliveryState{integrations.MessageSending, integrations.MessageReconciling}, integrations.MessageUncertain},
		} {
			if err := tx.Model(&database.IntegrationMessage{}).Where("category <> ? AND binding_id IN ? AND state IN ? AND next_attempt_at <= ?", integrations.CardCategory, live, recovery.states, now.Unix()).Update("state", recovery.next).Error; err != nil {
				return err
			}
		}

		if err := tx.Model(&database.IntegrationMessage{}).Where("category <> ? AND binding_id IN ? AND state = ? AND attempts >= ? AND next_attempt_at > 0", integrations.CardCategory, live, integrations.MessageUncertain, integrations.DeliveryAttempts).
			Updates(map[string]any{"next_attempt_at": 0, "error": "Delivery could not be confirmed after an interrupted attempt. Check delivery to look for the existing message."}).Error; err != nil {
			return err
		}

		if err := tx.Model(&database.IntegrationMessage{}).Where("category <> ? AND binding_id IN ? AND state = ? AND attempts >= ? AND next_attempt_at <= ?", integrations.CardCategory, live, model.DeliveryPending, integrations.DeliveryAttempts, now.Unix()).
			Updates(map[string]any{"state": model.DeliveryFailed, "error": "Delivery stopped after repeated interrupted attempts. Retry when the integration is available.", "next_attempt_at": 0}).Error; err != nil {
			return err
		}

		obsolete := tx.Model(&database.ServerIntegration{}).Select("id").Where("destination_id <> integration_messages.destination_id")
		if err := tx.Model(&database.IntegrationMessage{}).Where("category <> ? AND (state IN ? OR (state = ? AND next_attempt_at <= ?)) AND binding_id IN (?)", integrations.CardCategory, []model.DeliveryState{model.DeliveryPending, integrations.MessageRateLimited}, integrations.MessageGlobalLimited, now.Unix(), obsolete).
			Updates(map[string]any{"state": model.DeliveryFailed, "error": "The channel changed before this notification was sent.", "next_attempt_at": 0}).Error; err != nil {
			return err
		}

		states := []model.DeliveryState{model.DeliveryPending, integrations.MessageRateLimited, integrations.MessageGlobalLimited, integrations.MessageUncertain, integrations.MessageGlobalUncertain}

		var row database.IntegrationMessage

		query := tx.Table("integration_messages AS m").Select("m.*").
			Joins("JOIN server_integrations b ON b.id = m.binding_id JOIN integrations c ON c.id = b.integration_id").
			Where("m.category <> ?", integrations.CardCategory).
			Where("m.state IN ? AND m.next_attempt_at > 0 AND m.next_attempt_at <= ? AND (m.attempts < ? OR m.state IN ?) AND b.enabled = ? AND c.enabled = ?", states, now.Unix(), integrations.DeliveryAttempts, []model.DeliveryState{integrations.MessageRateLimited, integrations.MessageGlobalLimited, integrations.MessageGlobalUncertain}, true, true).
			Where("m.binding_id IN ?", live).Where(integrationDeliveryGate(clause.Column{Table: "m", Name: "destination_id"}, now)).
			Order(clause.OrderBy{Expression: clause.Expr{SQL: "CASE WHEN m.state IN ? THEN 0 ELSE 1 END, m.next_attempt_at, m.id", Vars: []any{[]model.DeliveryState{integrations.MessageUncertain, integrations.MessageGlobalUncertain}}}}).Limit(1).Find(&row)
		if query.Error != nil {
			return query.Error
		}

		if row.ID == 0 {
			return nil
		}

		store := NewIntegrationRepository(tx)

		binding, err := store.GetBinding(ctx, row.BindingID)
		if err != nil {
			return err
		}

		connection, err := store.GetConnection(ctx, binding.IntegrationID)
		if err != nil {
			return err
		}

		var server database.Server
		if err := tx.Select("name").Where("id = ?", binding.ServerID).Take(&server).Error; err != nil {
			return err
		}

		reconcile := row.State == integrations.MessageUncertain || row.State == integrations.MessageGlobalUncertain

		row.State = integrations.MessageClaimed
		if reconcile {
			row.State = integrations.MessageReconciling
		}

		row.Attempts++

		row.NextAttemptAt = now.Add(integrations.DeliveryLease).Unix()
		if err := tx.Model(&database.IntegrationMessage{}).Where("id = ?", row.ID).Updates(map[string]any{"state": row.State, "attempts": row.Attempts, "next_attempt_at": row.NextAttemptAt}).Error; err != nil {
			return err
		}

		claim = &IntegrationClaim{Message: integrationMessage(row), Connection: connection, ServerName: server.Name, Reconcile: reconcile}

		return nil
	})

	return claim, err
}

// BeginMessageSend rechecks current settings immediately before the external
// mutation. Only this transition makes an expired lease potentially ambiguous.
func (r *IntegrationRepository) BeginMessageSend(ctx context.Context, claim IntegrationClaim) error {
	return database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		store := NewIntegrationRepository(tx)

		binding, err := store.GetBinding(ctx, claim.BindingID)
		if err != nil {
			return err
		}

		connection, err := store.GetConnection(ctx, binding.IntegrationID)
		if err != nil {
			return err
		}

		if !binding.Enabled || !connection.Enabled {
			return ErrIntegrationDisabled
		}

		if binding.DestinationID != claim.DestinationID {
			return &integrations.DeliveryFailure{Message: "The channel changed before this notification was sent.", Permanent: true}
		}

		if connection.CredentialRef != claim.Connection.CredentialRef || !bytes.Equal(connection.Config, claim.Connection.Config) {
			return ErrIntegrationConflict
		}

		var event integrations.Event
		if json.Unmarshal(claim.Payload, &event) != nil || event.ServerID != binding.ServerID || !slices.ContainsFunc(binding.Subscriptions, func(subscription integrations.Subscription) bool { return subscription.Matches(event) }) {
			return &integrations.DeliveryFailure{Message: "The subscription changed before this notification was sent.", Permanent: true}
		}

		return integrationResult(tx.Model(&database.IntegrationMessage{}).Where("id = ? AND state = ? AND attempts = ? AND next_attempt_at = ?", claim.ID, integrations.MessageClaimed, claim.Attempts, claim.NextAttemptAt).Update("state", integrations.MessageSending))
	})
}

// FinishMessage retains a sanitized outcome only for the current lease owner.
func (r *IntegrationRepository) FinishMessage(ctx context.Context, claim IntegrationClaim, state model.DeliveryState, externalID, message string, next, now time.Time) error {
	return database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		if state == integrations.MessageCredentialsBlocked || state == integrations.MessageDestinationBlocked {
			current, err := NewIntegrationRepository(tx).GetConnection(ctx, claim.Connection.ID)
			if err != nil {
				return err
			}

			if current.CredentialRef != claim.Connection.CredentialRef || !bytes.Equal(current.Config, claim.Connection.Config) {
				state, message, next = model.DeliveryPending, "Saved integration settings changed; delivery will be checked again.", now.Add(5*time.Second)
			}
		}

		updates := map[string]any{"state": state, "error": message, "next_attempt_at": next.Unix()}
		if next.IsZero() {
			updates["next_attempt_at"] = 0
		}

		if externalID != "" {
			updates["external_message_id"], updates["delivered_at"] = externalID, now.Unix()
		}

		result := tx.Model(&database.IntegrationMessage{}).
			Where("id = ? AND state IN ? AND attempts = ? AND next_attempt_at = ?", claim.ID, []model.DeliveryState{integrations.MessageClaimed, integrations.MessageSending, integrations.MessageReconciling}, claim.Attempts, claim.NextAttemptAt).Updates(updates)
		if result.Error != nil {
			return result.Error
		}

		if result.RowsAffected != 1 {
			return ErrIntegrationConflict
		}

		return nil
	})
}

// IntegrationMessageStatus excludes payload data, credentials, and provider IDs.
type IntegrationMessageStatus struct {
	ID            int64                 `json:"id"`
	Category      integrations.Category `json:"category"`
	Title         string                `json:"title"`
	State         string                `json:"state"`
	Attempts      int                   `json:"attempts"`
	NextAttemptAt int64                 `json:"next_attempt_at"`
	DeliveredAt   int64                 `json:"delivered_at"`
	CreatedAt     int64                 `json:"created_at"`
	Error         string                `json:"error"`
	CanRetry      bool                  `json:"can_retry"`
}

// MessageStatuses returns bounded status pages without revealing event data.
func (r *IntegrationRepository) MessageStatuses(ctx context.Context, bindingID string, categories []integrations.Category, before int64, state string) ([]IntegrationMessageStatus, error) {
	if bindingID == "" {
		return []IntegrationMessageStatus{}, nil
	}

	binding, err := r.GetBinding(ctx, bindingID)
	if err != nil {
		return nil, err
	}

	connection, err := r.GetConnection(ctx, binding.IntegrationID)
	if err != nil {
		return nil, err
	}

	var rows []database.IntegrationMessage

	query := r.db.WithContext(ctx).Where("binding_id = ? AND category IN ? AND state <> ?", bindingID, categories, integrations.MessageReceipt)
	if before > 0 {
		query = query.Where("id < ?", before)
	}

	switch state {
	case "failed":
		query = query.Where("state IN ?", integrations.FailureStates())
	case "delivered":
		query = query.Where("state = ?", state)
	case "uncertain":
		query = query.Where("state IN ?", []model.DeliveryState{integrations.MessageUncertain, integrations.MessageGlobalUncertain, integrations.MessageReconciling})
	case "pending":
		query = query.Where("state IN ?", []model.DeliveryState{model.DeliveryPending, integrations.MessageClaimed, integrations.MessageSending, integrations.MessageRateLimited, integrations.MessageGlobalLimited})
	}

	if err := query.Order("id DESC").Limit(20).Find(&rows).Error; err != nil {
		return nil, err
	}

	result := make([]IntegrationMessageStatus, 0, len(rows))
	for _, row := range rows {
		var event integrations.Event

		_ = json.Unmarshal(row.Payload, &event)
		matches := event.Validate() == nil && slices.ContainsFunc(binding.Subscriptions, func(subscription integrations.Subscription) bool { return subscription.Matches(event) })

		nextAttempt := row.NextAttemptAt
		if row.State == integrations.MessageClaimed || row.State == integrations.MessageSending || row.State == integrations.MessageReconciling {
			nextAttempt = 0
		}

		retry := binding.Enabled && connection.Enabled && row.ExternalMessageID == "" && ((integrations.FailedMessage(row.State) && binding.DestinationID == row.DestinationID && matches) || ((row.State == integrations.MessageUncertain || row.State == integrations.MessageGlobalUncertain) && row.NextAttemptAt == 0))
		result = append(result, IntegrationMessageStatus{ID: row.ID, Category: row.Category, Title: event.Title, State: integrations.DeliveryState(row.State), Attempts: row.Attempts, NextAttemptAt: nextAttempt, DeliveredAt: row.DeliveredAt, CreatedAt: row.CreatedAt, Error: row.Error, CanRetry: retry})
	}

	return result, nil
}

// RetryMessage preserves uncertainty and fences retries by source and channel.
func (r *IntegrationRepository) RetryMessage(ctx context.Context, bindingID string, id int64, categories []integrations.Category, now time.Time) error {
	return database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		var row database.IntegrationMessage
		if err := tx.Where("id = ? AND binding_id = ? AND category IN ?", id, bindingID, categories).Take(&row).Error; err != nil {
			return integrationError(err)
		}

		if !integrations.FailedMessage(row.State) && row.State != integrations.MessageUncertain && row.State != integrations.MessageGlobalUncertain {
			return ErrIntegrationConflict
		}

		binding, err := NewIntegrationRepository(tx).GetBinding(ctx, bindingID)
		if err != nil {
			return err
		}

		connection, err := NewIntegrationRepository(tx).GetConnection(ctx, binding.IntegrationID)
		if err != nil {
			return err
		}

		if !binding.Enabled || !connection.Enabled {
			return ErrIntegrationDisabled
		}

		if row.ExternalMessageID != "" || (integrations.FailedMessage(row.State) && row.DestinationID != binding.DestinationID) {
			return ErrIntegrationConflict
		}

		if integrations.FailedMessage(row.State) {
			var event integrations.Event
			if json.Unmarshal(row.Payload, &event) != nil || event.Validate() != nil || !slices.ContainsFunc(binding.Subscriptions, func(subscription integrations.Subscription) bool { return subscription.Matches(event) }) {
				return ErrIntegrationConflict
			}
		}

		if !integrations.FailedMessage(row.State) && row.NextAttemptAt > now.Unix() {
			return ErrIntegrationConflict
		}

		state := model.DeliveryPending
		if !integrations.FailedMessage(row.State) {
			state = integrations.MessageUncertain
		}

		if err := NewIntegrationRepository(tx).ResumeMessages(ctx, binding.IntegrationID, row.DestinationID); err != nil {
			return err
		}

		return tx.Model(&row).Updates(map[string]any{"state": state, "attempts": 0, "next_attempt_at": now.Unix(), "error": ""}).Error
	})
}

// ResumeMessages clears operational pauses after validated settings or an
// explicit retry. It does not requeue other failed notifications.
func (r *IntegrationRepository) ResumeMessages(ctx context.Context, connectionID, destinationID string) error {
	bindings := r.db.Model(&database.ServerIntegration{}).Select("id").Where("integration_id = ?", connectionID)

	query := r.db.WithContext(ctx).Model(&database.IntegrationMessage{}).Where("binding_id IN (?)", bindings)
	if destinationID == "" {
		query = query.Where("state = ?", integrations.MessageCredentialsBlocked)
	} else {
		query = query.Where("(state = ? OR (state = ? AND destination_id = ?))", integrations.MessageCredentialsBlocked, integrations.MessageDestinationBlocked, destinationID)
	}

	return query.Update("state", model.DeliveryFailed).Error
}
