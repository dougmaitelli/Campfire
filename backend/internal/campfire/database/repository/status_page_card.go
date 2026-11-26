package repository

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"time"

	"campfire/internal/campfire/database"
	core "campfire/internal/campfire/integrations"
	"campfire/internal/campfire/statuspage"

	"gorm.io/gorm"
)

func savePageDestination(ctx context.Context, tx *gorm.DB, pageID string, channel *string, checked core.Connection) (*string, error) {
	store := NewIntegrationRepository(tx)

	binding, err := store.PageBinding(ctx, pageID)
	if err != nil && !errors.Is(err, ErrIntegrationNotFound) {
		return nil, err
	}

	if channel == nil {
		if binding.Enabled {
			return &binding.DestinationID, nil
		}

		if binding.ID != "" {
			return new(string), nil
		}

		return nil, nil
	}

	if *channel == "" {
		if binding.ID == "" {
			return nil, nil
		}

		if binding.Enabled {
			binding.Enabled = false
			if err := store.UpdateBinding(ctx, binding); err != nil {
				return nil, err
			}
		}

		return channel, prunePageBinding(tx, pageID, time.Now())
	}

	current, err := store.GetConnection(ctx, checked.ID)
	if err != nil {
		return nil, err
	}

	if !current.Enabled || current.CredentialRef != checked.CredentialRef || !bytes.Equal(current.Config, checked.Config) {
		return nil, ErrIntegrationConflict
	}

	if binding.ID == "" {
		binding = core.Binding{ID: rand.Text(), StatusPageID: pageID, IntegrationID: current.ID, DestinationID: *channel, Enabled: true, Card: core.CardSettings{Enabled: true}}
		err = store.CreateBinding(ctx, binding)
	} else {
		binding.DestinationID, binding.Enabled = *channel, true
		err = store.UpdateBinding(ctx, binding)
	}

	if err != nil {
		return nil, err
	}

	return channel, store.ResumeMessages(ctx, current.ID, *channel)
}

// PageBinding resolves only a status page's card destination.
func (r *IntegrationRepository) PageBinding(ctx context.Context, pageID string) (core.Binding, error) {
	var row database.ServerIntegration

	err := r.db.WithContext(ctx).Where("status_page_id = ?", pageID).Take(&row).Error

	return integrationBinding(row), integrationError(err)
}

// DiscordCardStatus returns delivery health without loading the public page or calling Discord.
func (r *StatusPageRepository) DiscordCardStatus(ctx context.Context, id string) (core.CardStatus, error) {
	store := NewIntegrationRepository(r.db)

	binding, err := store.PageBinding(ctx, id)
	if errors.Is(err, ErrIntegrationNotFound) {
		var page database.StatusPage

		err := r.db.WithContext(ctx).Select("id").Where("id = ?", id).Take(&page).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return core.CardStatus{}, statuspage.ErrNotFound
		}

		if err != nil {
			return core.CardStatus{}, err
		}

		return core.CardStatus{State: "disabled"}, nil
	}

	if err != nil {
		return core.CardStatus{}, err
	}

	connection, err := store.GetConnection(ctx, binding.IntegrationID)
	if err != nil {
		return core.CardStatus{}, err
	}

	return store.CardStatus(ctx, binding, connection)
}

// PageCardCleanup identifies a revoked page whose Discord message still needs attention.
type PageCardCleanup struct {
	ID        string          `json:"id"`
	Title     string          `json:"title"`
	ChannelID string          `json:"channel_id"`
	Status    core.CardStatus `json:"status"`
}

// DiscordCardCleanups reads retained receipts and their work in one snapshot.
func (r *StatusPageRepository) DiscordCardCleanups(ctx context.Context) ([]PageCardCleanup, error) {
	var rows []struct {
		database.IntegrationMessage             `gorm:"embedded"`
		PageID, PageTitle, ChannelID, ReceiptID string
	}

	err := r.db.WithContext(ctx).Table("server_integrations AS b").
		Select(`m.id, m.payload, m.state, m.error, m.next_attempt_at,
			b.status_page_id AS page_id, b.deleted_page_title AS page_title, b.external_message_id AS receipt_id,
			COALESCE(NULLIF(b.message_destination_id, ''), NULLIF(m.destination_id, ''), b.destination_id) AS channel_id`).
		Joins("LEFT JOIN integration_messages AS m ON m.binding_id = b.id AND m.category = ?", core.CardCategory).
		Where("b.status_page_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM status_pages p WHERE p.id = b.status_page_id)").
		Order("b.deleted_page_title, b.status_page_id").Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	result := make([]PageCardCleanup, 0, len(rows))
	for _, row := range rows {
		binding := core.Binding{StatusPageID: row.PageID, StatusMessage: core.MessageReference{ID: row.ReceiptID}}
		status := cardWorkStatus(binding, false, row.IntegrationMessage)
		result = append(result, PageCardCleanup{ID: row.PageID, Title: row.PageTitle, ChannelID: row.ChannelID, Status: status})
	}

	return result, nil
}

// RetryDiscordCard schedules the existing card for reconciliation.
func (r *StatusPageRepository) RetryDiscordCard(ctx context.Context, id string) error {
	store := NewIntegrationRepository(r.db)

	binding, err := store.PageBinding(ctx, id)
	if err != nil {
		return err
	}

	return store.RetryCard(ctx, binding.ID)
}

// DraftTargets resolves current placement while keeping the proposed names and order.
func (r *StatusPageRepository) DraftTargets(ctx context.Context, entries []statuspage.Selection) ([]PublishedTarget, error) {
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		ids = append(ids, e.ServerID)
	}

	var rows []PublishedTarget
	if len(ids) > 0 {
		if err := r.db.WithContext(ctx).Table("servers").Select("servers.id AS server_id, servers.spark_id, sparks.disabled").Joins("JOIN sparks ON sparks.id = servers.spark_id").Where("servers.id IN ? AND servers.deletion_pending = ?", ids, false).Scan(&rows).Error; err != nil {
			return nil, err
		}
	}

	byID := make(map[string]PublishedTarget, len(rows))
	for _, target := range rows {
		byID[target.ServerID] = target
	}

	targets := make([]PublishedTarget, 0, len(entries))
	for _, entry := range entries {
		target, ok := byID[entry.ServerID]
		if !ok {
			return nil, statuspage.ErrInput
		}

		target.Name = entry.Name
		targets = append(targets, target)
	}

	return targets, nil
}

// Leave the binding available for receipt recovery or cleanup after an in-flight send.
func prunePageBinding(tx *gorm.DB, pageID string, now time.Time) error {
	var control database.IntegrationMessage

	err := tx.Table("integration_messages AS m").Select("m.*").Joins("JOIN server_integrations b ON b.id = m.binding_id").Where("b.status_page_id = ? AND m.category = ?", pageID, core.CardCategory).Take(&control).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	if err == nil {
		work, err := decodeCardWork(control.Payload)
		if err != nil {
			return err
		}

		leased := control.State == core.CardClaimed || control.State == core.CardSending
		if work.Creating || (leased && control.NextAttemptAt > now.Unix()) {
			return nil
		}
	}

	return tx.Where("status_page_id = ? AND enabled = ? AND external_message_id = ''", pageID, false).
		Delete(&database.ServerIntegration{}).Error
}
