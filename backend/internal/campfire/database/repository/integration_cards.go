package repository

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"slices"
	"sort"
	"time"

	"campfire/internal/campfire/database"
	core "campfire/internal/campfire/integrations"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CardClaim fences one binding and destination across all panel processes.
type CardClaim struct {
	IntegrationClaim
	Binding core.Binding
	Work    core.CardWork
}

func cardEnabled(binding core.Binding, connection core.Connection) bool {
	return binding.Enabled && connection.Enabled && binding.Card.Enabled
}

func cardDestination(binding core.Binding, work core.CardWork, saved string) string {
	if binding.StatusMessage.ID != "" {
		return binding.StatusMessage.DestinationID
	}

	if work.Creating {
		return saved
	}

	return binding.DestinationID
}

// Ready work can wake from zero when settings were re-enabled while cleanup
// was already in flight. Failed or uncertain paused work still needs a retry.
func cardDue(row database.IntegrationMessage, active bool, now time.Time) bool {
	return row.NextAttemptAt <= now.Unix() && (row.NextAttemptAt != 0 || row.State == core.CardReady && active)
}

func decodeCardWork(payload json.RawMessage) (core.CardWork, error) {
	var work core.CardWork
	if json.Unmarshal(payload, &work) != nil || work.Version != 1 || (work.Creating && (work.CreationID == "" || work.CreatedAt <= 0)) {
		return work, errors.New("saved server-card state is invalid; automatic publishing is paused")
	}

	return work, nil
}

func cardFence(result *gorm.DB) error {
	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected != 1 {
		return ErrIntegrationConflict
	}

	return nil
}

// ClaimCard has a read-only idle path; card work is created only for enabled
// cards or bindings whose old message still needs cleanup.
func (r *IntegrationRepository) ClaimCard(ctx context.Context, now time.Time) (*CardClaim, error) {
	var bindings []database.ServerIntegration
	if err := r.db.WithContext(ctx).Preload("Integration").
		Where("id IN (SELECT binding_id FROM integration_messages WHERE category = ? AND next_attempt_at <= ? AND (next_attempt_at > 0 OR state = ?)) OR NOT EXISTS (SELECT 1 FROM integration_messages control WHERE control.binding_id = server_integrations.id AND control.event_id = ?)", core.CardCategory, now.Unix(), core.CardReady, core.CardEventID).
		Order("id").Find(&bindings).Error; err != nil {
		return nil, err
	}

	var messages []database.IntegrationMessage

	if len(bindings) == 0 {
		return nil, nil
	}

	ids := make([]string, len(bindings))
	for i, binding := range bindings {
		ids[i] = binding.ID
	}

	if err := r.db.WithContext(ctx).Where("category = ? AND binding_id IN ?", core.CardCategory, ids).Find(&messages).Error; err != nil {
		return nil, err
	}

	byBinding := make(map[string]database.IntegrationMessage, len(messages))
	for _, message := range messages {
		byBinding[message.BindingID] = message
	}

	sort.SliceStable(bindings, func(i, j int) bool {
		return byBinding[bindings[i].ID].NextAttemptAt < byBinding[bindings[j].ID].NextAttemptAt
	})

	for _, saved := range bindings {
		binding, connection := integrationBinding(saved), integrationConnection(saved.Integration)
		row, exists := byBinding[binding.ID]
		work := core.CardWork{Version: 1}

		if exists {
			if !cardDue(row, cardEnabled(binding, connection), now) {
				continue
			}

			var err error

			work, err = decodeCardWork(row.Payload)
			if err != nil {
				return nil, err
			}
		}

		if !cardEnabled(binding, connection) && binding.StatusMessage.ID == "" && !work.Creating {
			continue
		}

		var available int64
		if err := r.db.WithContext(ctx).Table("server_integrations AS b").Where("b.id = ?", binding.ID).Where(integrationDeliveryGate(cardDestination(binding, work, row.DestinationID), now)).Count(&available).Error; err != nil {
			return nil, err
		}

		if available == 0 {
			continue
		}

		var claim *CardClaim

		err := database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
			store := NewIntegrationRepository(tx)

			current, err := store.GetBinding(ctx, binding.ID)
			if err != nil {
				return err
			}

			conn, err := store.GetConnection(ctx, current.IntegrationID)
			if err != nil {
				return err
			}

			result := tx.Where("binding_id = ? AND event_id = ?", current.ID, core.CardEventID).Take(&row)
			if errors.Is(result.Error, gorm.ErrRecordNotFound) {
				if !cardEnabled(current, conn) && current.StatusMessage.ID == "" {
					return nil
				}

				payload, err := json.Marshal(core.CardWork{Version: 1})
				if err != nil {
					return err
				}

				row = database.IntegrationMessage{BindingID: current.ID, EventID: core.CardEventID, Category: core.CardCategory, DestinationID: current.DestinationID, Payload: payload, State: core.CardReady, NextAttemptAt: now.Unix(), CreatedAt: now.Unix()}
				if err := tx.Omit(clause.Associations).Create(&row).Error; err != nil {
					return err
				}
			} else if result.Error != nil {
				return result.Error
			}

			if !cardDue(row, cardEnabled(current, conn), now) {
				return nil
			}

			work, err := decodeCardWork(row.Payload)
			if err != nil {
				return err
			}

			if !cardEnabled(current, conn) && current.StatusMessage.ID == "" && !work.Creating {
				return nil
			}

			destination := cardDestination(current, work, row.DestinationID)

			if row.DestinationID != destination {
				row.DestinationID = destination
				if err := tx.Model(&database.IntegrationMessage{}).Where("id = ?", row.ID).Update("destination_id", destination).Error; err != nil {
					return err
				}
			}

			var free int64
			if err := tx.Table("integration_messages AS m").Joins("JOIN server_integrations b ON b.id = m.binding_id").Where("m.id = ?", row.ID).Where(integrationDeliveryGate(clause.Column{Table: "m", Name: "destination_id"}, now)).Count(&free).Error; err != nil {
				return err
			}

			if free == 0 {
				return nil
			}

			row.State, row.NextAttemptAt = core.CardClaimed, now.Add(core.DeliveryLease).Unix()

			row.Attempts++
			if err := tx.Model(&database.IntegrationMessage{}).Where("id = ?", row.ID).Updates(map[string]any{"state": row.State, "destination_id": row.DestinationID, "attempts": row.Attempts, "next_attempt_at": row.NextAttemptAt}).Error; err != nil {
				return err
			}

			claim = &CardClaim{IntegrationClaim: IntegrationClaim{Message: integrationMessage(row), Connection: conn}, Binding: current, Work: work}

			return nil
		})
		if err != nil || claim != nil {
			return claim, err
		}
	}

	return nil, nil
}

// BeginCardCreate durably records potential creation before the external POST.
// Settings are checked again so a disabled or moved binding cannot start a send.
func (r *IntegrationRepository) BeginCardCreate(ctx context.Context, claim *CardClaim, now time.Time) error {
	work := claim.Work
	if work.Creating {
		return ErrIntegrationConflict
	}

	err := database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		store := NewIntegrationRepository(tx)

		binding, err := store.GetBinding(ctx, claim.BindingID)
		if err != nil {
			return err
		}

		connection, err := store.GetConnection(ctx, binding.IntegrationID)
		if err != nil {
			return err
		}

		if !cardEnabled(binding, connection) || binding.DestinationID != claim.DestinationID || binding.StatusMessage.ID != "" || connection.CredentialRef != claim.Connection.CredentialRef || !bytes.Equal(connection.Config, claim.Connection.Config) || !slices.Equal(binding.Card.Fields, claim.Binding.Card.Fields) {
			return ErrIntegrationConflict
		}

		if work.CreationID == "" {
			work.CreationID = rand.Text()
		}

		work.Creating, work.CreatedAt = true, now.Unix()

		payload, err := json.Marshal(work)
		if err != nil {
			return err
		}

		return cardFence(tx.Model(&database.IntegrationMessage{}).Where("id = ? AND state = ? AND attempts = ? AND next_attempt_at = ?", claim.ID, core.CardClaimed, claim.Attempts, claim.NextAttemptAt).Updates(map[string]any{"state": core.CardSending, "payload": string(payload)}))
	})
	if err == nil {
		claim.Work = work
	}

	return err
}

// CheckCardMutation fences edits and cleanup against intervening settings saves.
func (r *IntegrationRepository) CheckCardMutation(ctx context.Context, claim CardClaim, deleting bool) error {
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

		active := cardEnabled(binding, connection) && binding.DestinationID == claim.DestinationID
		if binding.StatusMessage != claim.Binding.StatusMessage || connection.CredentialRef != claim.Connection.CredentialRef || !bytes.Equal(connection.Config, claim.Connection.Config) || (deleting && active) || (!deleting && (!active || !slices.Equal(binding.Card.Fields, claim.Binding.Card.Fields))) {
			return ErrIntegrationConflict
		}

		var owners int64
		if err := tx.Model(&database.IntegrationMessage{}).Where("id = ? AND state = ? AND attempts = ? AND next_attempt_at = ?", claim.ID, core.CardClaimed, claim.Attempts, claim.NextAttemptAt).Count(&owners).Error; err != nil {
			return err
		}

		if owners != 1 {
			return ErrIntegrationConflict
		}

		return nil
	})
}

// FinishCard atomically records the lifecycle outcome and its external reference.
// A newly created message remains owned even if settings moved while HTTP ran;
// the next pass cleans it up before creating in the new destination.
func (r *IntegrationRepository) FinishCard(ctx context.Context, claim CardClaim, work core.CardWork, ref core.MessageReference, state model.DeliveryState, message string, next time.Time) error {
	return database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		if state == core.MessageCredentialsBlocked || state == core.MessageDestinationBlocked {
			connection, err := NewIntegrationRepository(tx).GetConnection(ctx, claim.Connection.ID)
			if err != nil {
				return err
			}

			if connection.CredentialRef != claim.Connection.CredentialRef || !bytes.Equal(connection.Config, claim.Connection.Config) {
				state, message, next = core.CardReady, "Saved integration settings changed; the card will be checked again.", time.Now().Add(5*time.Second)
			}
		}

		payload, err := json.Marshal(work)
		if err != nil {
			return err
		}

		nextAt := int64(0)
		if !next.IsZero() {
			nextAt = next.Unix()
		}

		updates := map[string]any{"payload": string(payload), "state": state, "error": message, "next_attempt_at": nextAt}
		if state == core.CardReady && message == "" {
			updates["attempts"] = 0
		}

		result := tx.Model(&database.IntegrationMessage{}).Where("id = ? AND category = ? AND state IN ? AND attempts = ? AND next_attempt_at = ?", claim.ID, core.CardCategory, []model.DeliveryState{core.CardClaimed, core.CardSending}, claim.Attempts, claim.NextAttemptAt).Updates(updates)
		if err := cardFence(result); err != nil {
			return err
		}

		return cardFence(tx.Model(&database.ServerIntegration{}).Where("id = ? AND message_destination_id = ? AND external_message_id = ?", claim.BindingID, claim.Binding.StatusMessage.DestinationID, claim.Binding.StatusMessage.ID).Updates(map[string]any{"message_destination_id": ref.DestinationID, "external_message_id": ref.ID}))
	})
}

// WakeCards applies changed settings without clearing provider cooldowns or a
// creation identity. An ambiguous creation still requires receipt recovery.
func (r *IntegrationRepository) WakeCards(ctx context.Context, connectionID, bindingID string) error {
	query := r.db.WithContext(ctx).Model(&database.IntegrationMessage{}).Where("category = ? AND state IN ?", core.CardCategory, []model.DeliveryState{core.CardReady, model.DeliveryFailed, core.MessageCredentialsBlocked, core.MessageDestinationBlocked})
	if bindingID != "" {
		query = query.Where("binding_id = ?", bindingID)
	} else {
		query = query.Where("binding_id IN (?)", r.db.Model(&database.ServerIntegration{}).Select("id").Where("integration_id = ?", connectionID))
	}

	return query.Updates(map[string]any{"state": core.CardReady, "next_attempt_at": time.Now().Unix(), "attempts": 0, "error": ""}).Error
}

// CardStatus exposes lifecycle health without provider or creation identities.
func (r *IntegrationRepository) CardStatus(ctx context.Context, binding core.Binding, connection core.Connection) (core.CardStatus, error) {
	status := core.CardStatus{State: "disabled"}
	if cardEnabled(binding, connection) {
		status.State = "pending"
	}

	if binding.ID == "" {
		return status, nil
	}

	var row database.IntegrationMessage
	if err := r.db.WithContext(ctx).Where("binding_id = ? AND category = ?", binding.ID, core.CardCategory).Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return status, nil
		}

		return status, err
	}

	work, err := decodeCardWork(row.Payload)
	if err != nil {
		status.State, status.Error = "failed", "Saved card state is invalid. Automatic publishing is paused."
		return status, nil //nolint:nilerr // Invalid saved work is exposed as card health so settings remain readable.
	}

	if !cardEnabled(binding, connection) && binding.StatusMessage.ID == "" && !work.Creating {
		return status, nil
	}

	status.Error, status.NextAttemptAt = row.Error, row.NextAttemptAt

	status.CanRetry = row.NextAttemptAt == 0 && (row.Error != "" || work.Creating) && row.State != core.CardClaimed && row.State != core.CardSending
	if work.Creating {
		status.State = "uncertain"
	} else if core.FailedMessage(row.State) {
		status.State = "failed"
	} else if binding.StatusMessage.ID != "" && cardEnabled(binding, connection) && binding.StatusMessage.DestinationID == binding.DestinationID {
		status.State = "published"
	} else if binding.StatusMessage.ID != "" {
		status.State = "pending"
	}

	if row.State == core.CardClaimed || row.State == core.CardSending {
		status.NextAttemptAt = 0
	}

	return status, nil
}

// RetryCard retains creation identity and releases operational pauses explicitly.
func (r *IntegrationRepository) RetryCard(ctx context.Context, bindingID string) error {
	return database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		var row database.IntegrationMessage
		if err := tx.Where("binding_id = ? AND category = ?", bindingID, core.CardCategory).Take(&row).Error; err != nil {
			return integrationError(err)
		}

		if row.NextAttemptAt != 0 || row.State == core.CardClaimed || row.State == core.CardSending {
			return ErrIntegrationConflict
		}

		if _, err := decodeCardWork(row.Payload); err != nil {
			return ErrIntegrationConflict
		}

		store := NewIntegrationRepository(tx)

		binding, err := store.GetBinding(ctx, bindingID)
		if err != nil {
			return err
		}

		if err := store.ResumeMessages(ctx, binding.IntegrationID, row.DestinationID); err != nil {
			return err
		}

		return tx.Model(&row).Updates(map[string]any{"state": core.CardReady, "next_attempt_at": time.Now().Unix(), "attempts": 0, "error": ""}).Error
	})
}
