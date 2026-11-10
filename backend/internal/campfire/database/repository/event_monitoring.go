package repository

import (
	"context"
	"fmt"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/events"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

func monitoringAlertRecord(event events.MonitoringAlertChanged, at time.Time) database.MonitoringAlert {
	row := database.MonitoringAlert{ID: event.ID, TargetType: event.TargetType, TargetID: event.TargetID, Code: event.Code, Kind: model.NotificationAlert, Severity: model.NotificationWarning, Title: fmt.Sprintf("%s — %s alert", event.Name, event.Code), Body: event.Body, CreatedAt: at.Unix()}
	if event.Code == "health" || event.Code == "quota" {
		row.Severity = model.NotificationFailure
	}

	if !event.Active {
		row.Kind, row.Severity = model.NotificationRecovery, model.NotificationSuccess
		row.Title = fmt.Sprintf("%s — %s recovered", event.Name, event.Code)
		row.Body = "The " + event.Code + " condition has remained healthy for the configured recovery period."
	}

	return row
}

func (*EventPublisher) monitoringAlert(_ context.Context, tx *gorm.DB, event events.Event, at time.Time, _ *EventPublication) (bool, error) {
	changed, ok := event.(events.MonitoringAlertChanged)
	if !ok {
		return false, nil
	}

	if changed.ID == "" || changed.TargetID == "" || changed.Code == "" || (changed.TargetType != model.NotificationServer && changed.TargetType != model.NotificationSpark) {
		return true, ErrAccessInput
	}

	row := monitoringAlertRecord(changed, at)

	return true, tx.Create(&row).Error
}
