package repository

import (
	"context"
	"crypto/rand"
	"time"

	"campfire/internal/campfire/database"

	"gorm.io/gorm"
)

// ActivityRepository persists recurring schedules, executions and audit metadata.
type ActivityRepository struct{ db *gorm.DB }

// NewActivityRepository constructs activity storage on the panel database.
func NewActivityRepository(db *gorm.DB) *ActivityRepository { return &ActivityRepository{db: db} }

func visibleServers(tx *gorm.DB, userID int64) *gorm.DB {
	return tx.Model(&database.ServerOwnership{}).Select("server_id").Where("user_id = ?", userID).
		Or("server_id IN (?)", tx.Model(&database.ServerMember{}).Select("server_id").Where("user_id = ?", userID))
}

// Schedules lists only servers currently visible to the caller.
func (r *ActivityRepository) Schedules(ctx context.Context, userID int64, admin bool, serverID string) ([]database.Schedule, error) {
	items := []database.Schedule{}

	query := r.db.WithContext(ctx).Omit("Command").Order("created_at DESC, id DESC")
	if !admin {
		query = query.Where("server_id IN (?)", visibleServers(r.db.WithContext(ctx), userID))
	}

	if serverID != "" {
		query = query.Where("server_id = ?", serverID)
	}

	err := query.Limit(500).Find(&items).Error

	return items, err
}

// Schedule loads a definition privately, including its command for execution.
func (r *ActivityRepository) Schedule(ctx context.Context, id string) (database.Schedule, error) {
	var item database.Schedule

	err := r.db.WithContext(ctx).Take(&item, "id = ?", id).Error

	return item, accessError(err)
}

// SaveSchedule replaces a definition; editing explicitly assumes execution ownership.
func (r *ActivityRepository) SaveSchedule(ctx context.Context, item *database.Schedule, create bool) error {
	if create {
		return accessError(r.db.WithContext(ctx).Omit("Server").Create(item).Error)
	}

	result := r.db.WithContext(ctx).Model(&database.Schedule{}).Where("id = ? AND server_id = ? AND version = ?", item.ID, item.ServerID, item.Version-1).
		Select("Name", "Action", "Command", "IntervalMinutes", "NextRunAt", "Enabled", "UserID", "Username", "Version").Updates(item)
	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return ErrAccessConflict
	}

	return nil
}

// DeleteSchedule removes a recurring definition, preserving all execution records.
func (r *ActivityRepository) DeleteSchedule(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&database.Schedule{}, "id = ?", id).Error
}

// DisableSchedule prevents a revoked execution identity from repeatedly attempting work.
func (r *ActivityRepository) DisableSchedule(ctx context.Context, id string, version int64) error {
	return r.db.WithContext(ctx).Model(&database.Schedule{}).Where("id = ? AND version = ?", id, version).Update("enabled", false).Error
}

// EnqueueDue advances each due definition and queues at most one outstanding execution.
// Missed intervals are coalesced, never replayed as a burst after downtime.
func (r *ActivityRepository) EnqueueDue(ctx context.Context, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var items []database.Schedule
		if err := tx.Where("enabled = true AND next_run_at <= ?", now.Unix()).Order("next_run_at").Limit(100).Find(&items).Error; err != nil {
			return err
		}

		for _, item := range items {
			interval := item.IntervalMinutes * 60

			next := item.NextRunAt + ((now.Unix()-item.NextRunAt)/interval+1)*interval
			if err := tx.Model(&item).Update("next_run_at", next).Error; err != nil {
				return err
			}

			var count int64
			if err := tx.Model(&database.ScheduleRun{}).Where("schedule_id = ? AND state IN ('queued','dispatching','running')", item.ID).Count(&count).Error; err != nil {
				return err
			}

			if count != 0 {
				continue
			}

			var server database.Server
			if err := tx.Take(&server, "id = ?", item.ServerID).Error; err != nil {
				return err
			}

			run := database.ScheduleRun{ID: rand.Text(), ScheduleID: item.ID, ScheduleVersion: item.Version, Recurring: true, ServerID: item.ServerID, ServerName: server.Name, UserID: item.UserID, Username: item.Username, Action: item.Action, Content: item.Command, State: "queued", CreatedAt: now.Unix()}
			if err := tx.Create(&run).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

// CreateRun persists an execution before any node command is issued.
func (r *ActivityRepository) CreateRun(ctx context.Context, run *database.ScheduleRun) error {
	return r.db.WithContext(ctx).Create(run).Error
}

// Run loads one retained execution privately.
func (r *ActivityRepository) Run(ctx context.Context, id string) (database.ScheduleRun, error) {
	var run database.ScheduleRun

	err := r.db.WithContext(ctx).Take(&run, "id = ?", id).Error

	return run, accessError(err)
}

// Runs returns paginated execution history with current server visibility enforced.
func (r *ActivityRepository) Runs(ctx context.Context, userID int64, admin bool, serverID, before string) ([]database.ScheduleRun, error) {
	items := []database.ScheduleRun{}

	query := r.db.WithContext(ctx).Omit("Content").Order("created_at DESC, id DESC")
	if !admin {
		query = query.Where("server_id IN (?)", visibleServers(r.db.WithContext(ctx), userID))
	}

	if serverID != "" {
		query = query.Where("server_id = ?", serverID)
	}

	if before != "" {
		query = query.Where("(created_at, id) < (SELECT created_at, id FROM schedule_runs WHERE id = ?)", before)
	}

	err := query.Limit(100).Find(&items).Error

	return items, err
}

// Pending returns a bounded batch for the single panel worker.
func (r *ActivityRepository) Pending(ctx context.Context) ([]database.ScheduleRun, error) {
	items := []database.ScheduleRun{}
	err := r.db.WithContext(ctx).Where("state IN ('queued','running')").Order("polled_at, created_at, id").Limit(100).Find(&items).Error

	return items, err
}

// TouchRun rotates pending work fairly across the bounded worker pool.
func (r *ActivityRepository) TouchRun(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Model(&database.ScheduleRun{}).Where("id = ?", id).Update("polled_at", time.Now().UnixNano()).Error
}

// UpdateRun transitions an execution without allowing a terminal result to be overwritten.
func (r *ActivityRepository) UpdateRun(ctx context.Context, id, state, message string, now time.Time) error {
	updates := map[string]any{"state": state, "message": message}
	switch state {
	case "queued":
		updates["started_at"] = 0
	case "running", "dispatching":
		updates["started_at"] = now.Unix()
	default:
		updates["finished_at"] = now.Unix()
		updates["content"] = ""
	}

	return r.db.WithContext(ctx).Model(&database.ScheduleRun{}).Where("id = ? AND state IN ('queued','dispatching','running')", id).Updates(updates).Error
}

// FinishRun atomically retains a terminal result and its audit record exactly once.
func (r *ActivityRepository) FinishRun(ctx context.Context, run database.ScheduleRun, state, message string, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&database.ScheduleRun{}).Where("id = ? AND state IN ('queued','dispatching','running')", run.ID).Updates(map[string]any{"state": state, "message": message, "finished_at": now.Unix(), "content": ""})
		if result.Error != nil {
			return result.Error
		}

		if result.RowsAffected == 0 {
			return nil
		}

		return tx.Create(&database.AuditEvent{ID: rand.Text(), UserID: run.UserID, Username: run.Username, Action: "server." + run.Action, Target: "server:" + run.ServerID, Outcome: state, CreatedAt: now.Unix()}).Error
	})
}

// Recover marks uncertain in-flight operations explicitly instead of replaying mutations.
func (r *ActivityRepository) Recover(ctx context.Context, now time.Time) error {
	var runs []database.ScheduleRun
	if err := r.db.WithContext(ctx).Where("state IN ('running','dispatching')").Find(&runs).Error; err != nil {
		return err
	}

	for _, run := range runs {
		if err := r.FinishRun(ctx, run, "interrupted", "Panel restarted; verify server state before retrying.", now); err != nil {
			return err
		}
	}

	return r.db.WithContext(ctx).Model(&database.AuditEvent{}).Where("outcome = 'pending'").Update("outcome", "interrupted").Error
}

// BeginAudit durably records intent before an authenticated mutation.
func (r *ActivityRepository) BeginAudit(ctx context.Context, event *database.AuditEvent) error {
	return r.db.WithContext(ctx).Create(event).Error
}

// FinishAudit records only allowlisted action, target, actor and outcome metadata.
func (r *ActivityRepository) FinishAudit(ctx context.Context, event database.AuditEvent) error {
	return r.db.WithContext(ctx).Model(&database.AuditEvent{}).Where("id = ?", event.ID).
		Select("UserID", "Username", "Action", "Target", "Outcome", "Status").Updates(event).Error
}

// Audit lists administrator-only metadata with bounded pagination and optional filters.
func (r *ActivityRepository) Audit(ctx context.Context, before, action, target string) ([]database.AuditEvent, error) {
	items := []database.AuditEvent{}

	query := r.db.WithContext(ctx).Order("created_at DESC, id DESC")
	if before != "" {
		query = query.Where("(created_at, id) < (SELECT created_at, id FROM audit_events WHERE id = ?)", before)
	}

	if action != "" {
		query = query.Where("action = ?", action)
	}

	if target != "" {
		query = query.Where("target = ?", target)
	}

	err := query.Limit(100).Find(&items).Error

	return items, err
}
