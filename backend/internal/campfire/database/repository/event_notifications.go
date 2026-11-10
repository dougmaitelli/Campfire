package repository

import (
	"context"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/events"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

// panelNotification owns which application events create an inbox notification.
// Game, power, and operation events continue to use only their configured
// integration subscriptions. Console alerts retain safe, author-written guidance.
func (p *EventPublisher) panelNotification(_ context.Context, tx *gorm.DB, event events.Event, at time.Time, publication *EventPublication) (bool, error) {
	var item database.Notification

	switch event := event.(type) {
	case events.MonitoringAlertChanged:
		alert := monitoringAlertRecord(event, at)
		item = database.Notification{SourceKey: "monitoring_alert:" + alert.ID, TargetType: alert.TargetType, TargetID: alert.TargetID, Kind: alert.Kind, Severity: alert.Severity, Title: alert.Title, Body: alert.Body, CreatedAt: alert.CreatedAt}
	case events.ConsoleAlertDetected:
		severity := model.NotificationWarning
		if event.Alert.Severity == model.LogError {
			severity = model.NotificationFailure
		}

		item = database.Notification{SourceKey: "console_alert:" + event.Alert.ID, TargetType: model.NotificationServer, TargetID: event.ServerID, Kind: model.NotificationAlert, Severity: severity, Title: event.ServerName + ": " + event.Alert.Title, Body: event.Alert.Message, CreatedAt: at.Unix()}
	case events.NotificationRequested:
		item = database.Notification{ID: event.ID, SourceKey: event.SourceKey, TargetType: event.TargetType, TargetID: event.TargetID, Kind: event.Kind, Severity: event.Severity, Title: event.Title, Body: event.Body, CreatedAt: at.Unix()}
	default:
		return false, nil
	}

	if !events.ValidSourceKey(item.SourceKey) {
		return true, ErrAccessInput
	}

	if err := storeNotification(tx, &item, p.queueExternal); err != nil {
		return true, err
	}

	publication.NotificationID = item.ID

	return true, nil
}
