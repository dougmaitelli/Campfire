package repository

import (
	"context"
	"strconv"
	"time"

	"campfire/internal/campfire/events"
	"campfire/internal/campfire/integrations"

	"gorm.io/gorm"
)

// integrationMessages translates application facts into provider-neutral events.
// Binding/subscription matching, deduplication, and delivery stay in the existing
// integration router and durable queue. Inbox notifications are not forwarded here.
func (*EventPublisher) integrationMessages(ctx context.Context, tx *gorm.DB, event events.Event, at time.Time, _ *EventPublication) (bool, error) {
	var outgoing integrations.Event

	switch event := event.(type) {
	case events.GameEventRecorded:
		outgoing = integrations.GameEvent(event.ServerID, event.Event)
		outgoing.SourceKey = events.GameEventSourceKey(event.RecordID)
	case events.GameMediaRecorded:
		outgoing = integrations.GameMediaEvent(event.ServerID, event.Event, event.RecordID, event.Media)
	case events.ConsoleAlertDetected:
		outgoing = integrations.ConsoleAlert(event.ServerID, event.Alert)
	case events.OperationFinished:
		var applicable bool

		outgoing, applicable = integrations.OperationResult(event.ServerID, event.OperationID, event.Action, event.State, event.BackupMode, at)
		if !applicable {
			return true, nil
		}
	case events.PowerStateChanged:
		kind := "server." + string(event.Current)
		if event.Restarted {
			kind = "server.restarted"
		}

		outgoing = integrations.Event{ID: "power:" + strconv.FormatInt(at.Unix(), 10), ServerID: event.ServerID, Category: integrations.PowerEvents, Type: kind, Title: "Server state: " + string(event.Current), OccurredAt: at.Unix(), Data: map[string]any{"state": string(event.Current), "previous_state": string(event.Previous)}}
	default:
		return false, nil
	}

	return true, NewIntegrationRepository(tx).RouteEvent(ctx, outgoing, at)
}
