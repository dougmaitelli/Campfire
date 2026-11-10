package repository

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/events"
	"campfire/internal/campfire/integrations"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

// ActivityNotification exposes safe delivery information without credentials,
// worker leases, per-recipient read state or raw transport responses.
type ActivityNotification struct {
	BindingID     string                        `json:"binding_id,omitempty"`
	MessageID     int64                         `json:"message_id"`
	Name          string                        `json:"name"`
	Body          string                        `json:"body"`
	Severity      string                        `json:"severity,omitempty"`
	Data          map[string]any                `json:"data,omitempty"`
	FieldLabels   model.GameFieldLabels         `json:"field_labels,omitempty"`
	Attachment    *integrations.EventAttachment `json:"attachment,omitempty"`
	Attempts      int                           `json:"attempts"`
	NextAttemptAt int64                         `json:"next_attempt_at"`
	DeliveredAt   int64                         `json:"delivered_at"`
	Error         string                        `json:"error"`
	CanRetry      bool                          `json:"can_retry"`
	ExternalURL   string                        `json:"external_url,omitempty"`
}

// ServerActivityDetails expands one authorized activity entry.
type ServerActivityDetails struct {
	ServerActivityEntry
	GameEvent       *database.GameEvent       `json:"game_event,omitempty"`
	Alert           *model.DetectedLogAlert   `json:"alert,omitempty"`
	MonitoringAlert *database.MonitoringAlert `json:"monitoring_alert,omitempty"`
	Operation       *database.ScheduleRun     `json:"operation,omitempty"`
	Job             *JobHistory               `json:"-"`
	Notification    *ActivityNotification     `json:"notification,omitempty"`
}

func (r *ServerActivityRepository) entry(ctx context.Context, serverID, key string, access ServerActivityAccess) (ServerActivityEntry, error) {
	if len(key) > 256 || !strings.Contains(key, ":") {
		return ServerActivityEntry{}, ErrAccessNotFound
	}

	kind, id, _ := strings.Cut(key, ":")
	queries := r.rootsByKeys(ctx, serverID, access, []string{key})

	if kind == "notification" || kind == "integration_message" {
		number, err := strconv.ParseInt(id, 10, 64)
		if err != nil || number < 1 {
			return ServerActivityEntry{}, ErrAccessNotFound
		}

		notifications := r.notifications(ctx, serverID, access)
		if kind == "notification" {
			queries = append(queries, notifications.panel.Where("notifications.id = ?", number))
		} else {
			queries = append(queries, notifications.outgoing.Where("m.id = ?", number))
		}
	}

	if len(queries) == 0 {
		return ServerActivityEntry{}, ErrAccessNotFound
	}

	var entries []ServerActivityEntry
	if err := activityUnion(r.db.WithContext(ctx), queries).Where("key = ?", key).Limit(1).Scan(&entries).Error; err != nil {
		return ServerActivityEntry{}, err
	}

	if len(entries) == 0 {
		return ServerActivityEntry{}, ErrAccessNotFound
	}

	return entries[0], nil
}

// Details locates the item through the same authorized projection used by List.
func (r *ServerActivityRepository) Details(ctx context.Context, serverID, key string, access ServerActivityAccess) (details ServerActivityDetails, resultErr error) {
	defer func() {
		if errors.Is(resultErr, gorm.ErrRecordNotFound) {
			resultErr = ErrAccessNotFound
		}
	}()

	entry, err := r.entry(ctx, serverID, key, access)
	if err != nil {
		return details, err
	}

	entries := []ServerActivityEntry{entry}
	if err := r.enrichEntries(ctx, serverID, access, entries); err != nil {
		return details, err
	}

	details.ServerActivityEntry = entries[0]
	db := r.db.WithContext(ctx)
	kind, id, _ := strings.Cut(key, ":")

	switch kind {
	case "monitoring_alert":
		var alert database.MonitoringAlert
		if err := db.Where("target_type = ? AND target_id = ? AND id = ?", model.NotificationServer, serverID, id).Take(&alert).Error; err != nil {
			return details, err
		}

		details.MonitoringAlert = &alert
	case "game_event":
		var event database.GameEvent
		if err := db.Preload("Attachments", func(tx *gorm.DB) *gorm.DB { return tx.Omit("Data").Order("id") }).Where("server_id = ? AND id = ?", serverID, id).Take(&event).Error; err != nil {
			return details, err
		}

		labels, err := readGameEventLabels(db, serverID, event.Type)
		if err != nil {
			return details, err
		}

		event.FieldLabels = labelsForGameFields(labels[event.Type], event.Data)
		details.GameEvent = &event
	case "console_alert":
		var issue database.LogAlertIssue
		if err := db.Where("server_id = ? AND id = ?", serverID, id).Take(&issue).Error; err != nil {
			return details, err
		}

		if !access.Console {
			issue.Detection.Line = ""
		}

		details.Alert = &issue.Detection
	case "operation":
		var run database.ScheduleRun
		if err := db.Omit("Content").Where("server_id = ? AND id = ?", serverID, id).Take(&run).Error; err != nil {
			return details, err
		}

		details.Operation = &run
	case "job":
		job, err := NewJobRepository(db).HistoryItem(ctx, id)
		if err != nil {
			return details, err
		}

		details.Job = &job
	case "notification":
		var item database.Notification
		if err := db.Where("target_type = ? AND target_id = ? AND id = ?", model.NotificationServer, serverID, id).Take(&item).Error; err != nil {
			return details, err
		}

		notification := &ActivityNotification{MessageID: item.ID, Name: "Campfire", Body: item.Body, Severity: string(item.Severity)}

		var delivery database.NotificationDelivery
		if err := db.Where("notification_id = ?", item.ID).Take(&delivery).Error; err == nil {
			notification.Name = "Campfire → Apprise"
			notification.Attempts, notification.NextAttemptAt, notification.DeliveredAt = delivery.Attempts, delivery.NextAttemptAt, delivery.DeliveredAt
			notification.Error, notification.CanRetry = delivery.Error, access.ManageNotifications && delivery.State == model.DeliveryFailed
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return details, err
		}

		details.Notification = notification
	case "integration_message":
		var row database.IntegrationMessage
		if err := db.Where("id = ?", id).Take(&row).Error; err != nil {
			return details, err
		}

		store := NewIntegrationRepository(db)

		binding, connection, err := activityMessageOwner(ctx, store, row.BindingID)
		if err != nil {
			return details, err
		}

		status := IntegrationMessageStatusFor(row, binding, connection)

		var event integrations.Event

		_ = json.Unmarshal(row.Payload, &event)
		details.Notification = &ActivityNotification{BindingID: row.BindingID, MessageID: row.ID, Name: connection.Name, Body: event.Body, Data: event.Data, FieldLabels: event.FieldLabels, Attachment: event.Attachment, Attempts: status.Attempts, NextAttemptAt: status.NextAttemptAt, DeliveredAt: status.DeliveredAt, Error: status.Error, CanRetry: status.CanRetry && access.ManageIntegrations}

		var config struct {
			GuildID string `json:"guild_id"`
		}
		if connection.Provider == "discord" && json.Unmarshal(connection.Config, &config) == nil && activityDiscordID(config.GuildID) && activityDiscordID(row.DestinationID) && activityDiscordID(row.ExternalMessageID) {
			details.Notification.ExternalURL = "https://discord.com/channels/" + url.PathEscape(config.GuildID) + "/" + url.PathEscape(row.DestinationID) + "/" + url.PathEscape(row.ExternalMessageID)
		}
	}

	return details, nil
}

func (r *ServerActivityRepository) rootsByKeys(ctx context.Context, serverID string, access ServerActivityAccess, keys []string) []*gorm.DB {
	ids := map[string][]string{}

	for _, key := range keys {
		if key == "" || !events.ValidSourceKey(key) {
			continue
		}

		kind, id, _ := strings.Cut(key, ":")
		ids[kind] = append(ids[kind], id)
	}

	queries := r.roots(ctx, serverID, access)

	result := make([]*gorm.DB, 0, len(ids))
	for _, source := range []struct {
		query *gorm.DB
		ids   []string
	}{
		{queries.game, ids["game_event"]},
		{queries.alerts, ids["console_alert"]},
		{queries.operations, ids["operation"]},
		{queries.jobs, ids["job"]},
		{queries.monitoringAlerts, ids["monitoring_alert"]},
	} {
		if len(source.ids) > 0 {
			result = append(result, source.query.Where("id IN ?", source.ids))
		}
	}

	return result
}

func activityMessageOwner(ctx context.Context, store *IntegrationRepository, bindingID string) (integrations.Binding, integrations.Connection, error) {
	binding, err := store.GetBinding(ctx, bindingID)
	if err != nil {
		return binding, integrations.Connection{}, err
	}

	connection, err := store.GetConnection(ctx, binding.IntegrationID)

	return binding, connection, err
}

func activityDiscordID(value string) bool {
	if len(value) == 0 || len(value) > 20 {
		return false
	}

	_, err := strconv.ParseUint(value, 10, 64)

	return err == nil && !strings.ContainsAny(value, "+-")
}

// RetryPanelDelivery requeues exactly one server's failed Apprise delivery.
// Callers must enforce the existing NotificationsManage platform permission.
func (r *ServerActivityRepository) RetryPanelDelivery(ctx context.Context, serverID string, id int64, now time.Time) error {
	result := r.db.WithContext(ctx).Model(&database.NotificationDelivery{}).Where("notification_id = ? AND state = ? AND notification_id IN (?)", id, model.DeliveryFailed, r.db.Model(&database.Notification{}).Select("id").Where("target_type = ? AND target_id = ?", model.NotificationServer, serverID)).Updates(map[string]any{"state": model.DeliveryPending, "attempts": 0, "next_attempt_at": now.Unix(), "delivered_at": 0, "error": ""})
	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected != 1 {
		return ErrAccessConflict
	}

	return nil
}

// FilterChoices discovers only choices used by the selected history view.
func (r *ServerActivityRepository) FilterChoices(ctx context.Context, serverID, kind string) (ServerActivityChoices, error) {
	choices := ServerActivityChoices{Game: GameEventFilterOptions{Types: []string{}, Fields: map[string][]string{}}}
	if err := (ServerActivityFilter{Kind: kind}).Validate(); err != nil {
		return choices, err
	}

	if kind == "game" {
		game, err := r.cachedGameChoices(ctx, serverID)
		choices.Game = game

		return choices, err
	}

	return choices, nil
}

// ServerActivityChoices supplies dynamic game filter options.
type ServerActivityChoices struct {
	Game GameEventFilterOptions `json:"game"`
}
