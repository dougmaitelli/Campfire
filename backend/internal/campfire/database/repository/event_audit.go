package repository

import (
	"context"
	"crypto/rand"
	"errors"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/events"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

func (*EventPublisher) auditEvent(_ context.Context, tx *gorm.DB, event events.Event, at time.Time, _ *EventPublication) (bool, error) {
	var row database.AuditEvent

	switch event := event.(type) {
	case events.OperationFinished:
		var actorID *int64

		if event.UserID != 0 {
			var user database.User

			err := tx.Unscoped().Select("id").Where("id = ?", event.UserID).Take(&user).Error
			if err == nil {
				actorID = &user.ID
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return true, err
			}
		}

		row = database.AuditEvent{ID: rand.Text(), UserID: actorID, Username: event.Username, Action: model.ResourceAuditAction("server", string(event.Action)), Target: "server:" + event.ServerID, Outcome: model.AuditOutcome(event.State), CreatedAt: at.Unix()}
	case events.JobStateChanged:
		var outcome model.AuditOutcome

		switch event.Current {
		case model.JobCompleted:
			outcome = model.AuditSucceeded
		case model.JobCancelled:
			outcome = model.AuditCancelled
		case model.JobFailed:
			outcome = model.AuditFailed
		default:
			return true, nil
		}

		row = database.AuditEvent{ID: rand.Text(), Action: model.ResourceAuditAction("job."+string(event.Type), string(event.Current)), Target: "server:" + event.SourceServerID, Outcome: outcome, CreatedAt: at.Unix()}
	case events.RequestStarted:
		row = requestAudit(event, at)
	case events.RequestFinished:
		row = requestAudit(event.RequestStarted, at)
		return true, tx.Model(&database.AuditEvent{}).Where("id = ?", row.ID).Select("UserID", "Username", "APITokenID", "Action", "Target", "Outcome", "Status").Updates(row).Error
	default:
		return false, nil
	}

	return true, tx.Create(&row).Error
}

func requestAudit(event events.RequestStarted, at time.Time) database.AuditEvent {
	return database.AuditEvent{ID: event.ID, UserID: event.UserID, Username: event.Username, APITokenID: event.APITokenID, Action: event.Action, Target: event.Target, Outcome: event.Outcome, Status: event.Status, CreatedAt: at.Unix()}
}

func requestEvent(row database.AuditEvent) events.RequestStarted {
	return events.RequestStarted{ID: row.ID, UserID: row.UserID, Username: row.Username, APITokenID: row.APITokenID, Action: row.Action, Target: row.Target, Outcome: row.Outcome, Status: row.Status}
}
