//go:build !production

package devmock

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

const historySeedMarkerID = 15
const historyExampleCount = 125
const historyExamplePrefix = "mock-pagination-"
const historyArchivedServerID = "mock-pagination-retired-server"
const historyRetiredScheduleID = "mock-pagination-retired-schedule"

// EnableHistoryExamples adds multi-page history only to the interactive mock.
// Call before background workers start. A marker preserves edits and deletions
// across rebuilds; the shared devfixture and browser screenshot seed stay small.
func (r *Runtime) EnableHistoryExamples(ctx context.Context) error {
	return database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&seedMarker{}).Where("id = ?", historySeedMarkerID).Count(&count).Error; err != nil || count > 0 {
			return err
		}

		var rows []database.Server
		if err := tx.Select("id", "name", "spark_id").Where("id IN ?", []string{"survival", "creative", "modded"}).Find(&rows).Error; err != nil {
			return err
		}

		servers := make(map[string]database.Server, len(rows))
		for _, row := range rows {
			servers[row.ID] = row
		}

		now := r.now().UTC()
		for _, seed := range []func(context.Context, *gorm.DB, time.Time, map[string]database.Server) error{
			seedHistoryJobs, seedHistoryExecutions, seedHistoryNotifications, seedHistoryGameEvents, seedHistoryAudit,
		} {
			if err := seed(ctx, tx, now, servers); err != nil {
				return err
			}
		}

		return tx.Create(&seedMarker{ID: historySeedMarkerID}).Error
	})
}

func historyExampleTime(now time.Time, index int) time.Time {
	// Keep the existing actionable examples ahead of this older, terminal history.
	return now.Add(-12*time.Hour - time.Duration(index)*time.Minute)
}

func seedHistoryJobs(_ context.Context, tx *gorm.DB, now time.Time, servers map[string]database.Server) error {
	ids := []string{"survival", "creative", "modded"}
	for i := range historyExampleCount {
		source, sourceExists := servers[ids[i%len(ids)]]

		target, targetExists := servers[ids[(i+1)%len(ids)]]
		if !sourceExists || !targetExists {
			continue
		}

		at := historyExampleTime(now, i).Unix()
		finished := at + 120

		job := database.ServerJob{ID: fmt.Sprintf("%sjob-%03d", historyExamplePrefix, i+1), Type: model.JobCopy, Status: model.JobCompleted, Step: model.JobCleanup, Attempt: 1, Data: "{}", SourceServerID: source.ID, TargetServerID: target.ID, SourceSparkID: source.SparkID, TargetSparkID: target.SparkID, CreatedAt: at, UpdatedAt: finished, FinishedAt: &finished, TotalBytes: 64 << 20, TransferredBytes: 64 << 20}
		if i%3 == 0 {
			job.Type = model.JobTransfer
			job.TargetServerID = source.ID

			job.TargetSparkID = "mock-europe"
			if source.SparkID == job.TargetSparkID {
				job.TargetSparkID = "mock-america"
			}
		}

		if i%4 == 0 {
			job.Status = model.JobCancelled
			job.Step = model.JobCopying
			job.TransferredBytes = 16 << 20
		}
		// Only terminal jobs: bulk fixtures must never reserve servers or run work.
		if err := tx.Create(&job).Error; err != nil {
			return err
		}
	}

	return nil
}

func seedHistoryExecutions(_ context.Context, tx *gorm.DB, now time.Time, servers map[string]database.Server) error {
	states := []model.ExecutionState{model.ExecutionSucceeded, model.ExecutionFailed, model.ExecutionInterrupted, model.ExecutionCancelled, model.ExecutionSkipped}

	// 65 nightly restarts, 40 backups, 10 retired-schedule runs, and 10
	// archived-server runs. The nightly drawer must have its own second page.
	for i := range historyExampleCount {
		serverID, scheduleID := "survival", "nightly-restart"
		action := model.ServerActionRestart

		switch {
		case i >= 115:
			serverID, scheduleID = historyArchivedServerID, "mock-pagination-archived-schedule"
		case i >= 105:
			scheduleID = historyRetiredScheduleID
		case i >= 65:
			serverID, scheduleID, action = "creative", "savedata-backup", model.ServerActionBackup
		}

		server, exists := servers[serverID]
		if !exists && serverID != historyArchivedServerID {
			continue
		}

		name := server.Name
		if serverID == historyArchivedServerID {
			name = "Retired realm · Pagination"
		}

		at := historyExampleTime(now, i).Unix()

		run := database.ScheduleRun{ID: fmt.Sprintf("%sexecution-%03d", historyExamplePrefix, i+1), ServerID: serverID, ServerName: name, ScheduleID: scheduleID, Recurring: serverID == "survival" && scheduleID == "nightly-restart", UserID: 1, Username: Username, Action: action, State: states[i%len(states)], Message: "Pagination history example", CreatedAt: at, StartedAt: at, FinishedAt: at + 60}
		if action == model.ServerActionBackup {
			run.BackupMode = model.BackupModeSavedata
		}

		if err := tx.Create(&run).Error; err != nil {
			return err
		}
	}

	return nil
}

func seedHistoryNotifications(ctx context.Context, tx *gorm.DB, now time.Time, servers map[string]database.Server) error {
	inbox := repository.NewNotificationRepository(tx)
	severities := []model.NotificationSeverity{model.NotificationInfo, model.NotificationWarning, model.NotificationFailure}

	for i := range historyExampleCount {
		targetType, targetID := model.NotificationAccount, "1"
		if i%3 != 0 {
			targetType, targetID = model.NotificationServer, "survival"
			if _, exists := servers[targetID]; !exists {
				continue
			}
		}

		at := historyExampleTime(now, i).Unix()
		item := database.Notification{TargetType: targetType, TargetID: targetID, Kind: model.NotificationSchedule, Severity: severities[(i/3)%len(severities)], Title: fmt.Sprintf("Pagination message %03d", i+1), Body: "Historical mock notification for paging, read/unread actions, and Mark all read on older pages.", CreatedAt: at}
		// Inbox-only publications never queue Apprise or Discord deliveries.
		if err := inbox.Emit(ctx, &item, false); err != nil {
			return err
		}

		if i%4 == 0 {
			if err := tx.Model(&database.NotificationRecipient{}).Where("notification_id = ?", item.ID).Update("read_at", at+60).Error; err != nil {
				return err
			}
		}
	}

	return nil
}

func seedHistoryGameEvents(_ context.Context, tx *gorm.DB, now time.Time, servers map[string]database.Server) error {
	server, exists := servers["survival"]
	if !exists {
		return nil
	}

	for i := range historyExampleCount {
		at := historyExampleTime(now, i)

		event := database.GameEvent{ServerID: server.ID, SparkID: server.SparkID, EventID: fmt.Sprintf("%sevent-%03d", historyExamplePrefix, i+1), Type: "player.joined", Title: fmt.Sprintf("Pagination · Alex joined %03d", i+1), OccurredAt: at, ReceivedAt: at.Unix(), Data: map[string]any{"player": "Alex", "state": "connected"}}
		if i%3 == 0 {
			event.Type = "minecraft.advancement"
			event.Title = fmt.Sprintf("Pagination · Sam earned Stone Age %03d", i+1)
			event.Data = map[string]any{"player": "Sam", "advancement": "Stone Age"}
		}
		// Insert historical records, not new publications: do not trigger integrations
		// or pruning of a developer's existing events when upgrading a saved mock DB.
		if err := tx.Omit("Server", "Attachments").Create(&event).Error; err != nil {
			return err
		}
	}

	return nil
}

func seedHistoryAudit(ctx context.Context, tx *gorm.DB, now time.Time, servers map[string]database.Server) error {
	activity := repository.NewActivityRepository(tx)
	actor := int64(1)

	for i := range historyExampleCount {
		serverID := "survival"
		if i%4 == 0 {
			serverID = "creative"
		}

		if _, exists := servers[serverID]; !exists {
			continue
		}

		action := model.ServerActionStart
		if i%3 == 0 {
			action = model.ServerActionRestart
		}

		outcome, status := model.AuditSucceeded, http.StatusAccepted
		if i%5 == 0 {
			outcome, status = model.AuditFailed, http.StatusServiceUnavailable
		}

		event := database.AuditEvent{ID: fmt.Sprintf("%saudit-%03d", historyExamplePrefix, i+1), UserID: &actor, Username: Username, Action: model.ResourceAuditAction("server", string(action)), Target: "server:" + serverID, Outcome: outcome, Status: status, CreatedAt: historyExampleTime(now, i).Unix()}
		if err := activity.BeginAudit(ctx, &event); err != nil {
			return err
		}
	}

	return nil
}
