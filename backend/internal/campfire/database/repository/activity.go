package repository

import (
	"context"
	"crypto/rand"
	"errors"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/recurrence"
	accessperm "campfire/internal/shared/permissions"

	"gorm.io/gorm"
)

// ActivityRepository persists recurring schedules, executions and audit metadata.
type ActivityRepository struct{ db *gorm.DB }

// NewActivityRepository constructs activity storage on the panel database.
func NewActivityRepository(db *gorm.DB) *ActivityRepository { return &ActivityRepository{db: db} }

func scheduleUsers(tx *gorm.DB) *gorm.DB {
	return tx.Model(&database.Schedule{}).Select("schedules.*, COALESCE(users.username, '') AS username").Joins("LEFT JOIN users ON users.id = schedules.run_as_user_id")
}

func visibleServers(tx *gorm.DB, userID int64) *gorm.DB {
	return tx.Model(&database.Server{}).Select("servers.id").Where("owner_user_id = ?", userID).
		Or("servers.id IN (?)", tx.Model(&database.ServerAccessGrant{}).Select("server_id").Joins("JOIN role_permissions ON role_permissions.role_id = server_access_grants.access_role_id").Where("permission = ? AND (user_id = ? OR server_access_grants.role_id IN (SELECT role_id FROM users WHERE id = ?))", accessperm.ServerView, userID, userID))
}

// Schedules lists only servers currently visible to the caller.
func (r *ActivityRepository) Schedules(ctx context.Context, userID int64, admin bool, serverID string) ([]database.Schedule, error) {
	items := []database.Schedule{}

	query := scheduleUsers(r.db.WithContext(ctx)).Order("schedules.created_at DESC, schedules.id DESC")
	if !admin {
		query = query.Where("schedules.server_id IN (?)", visibleServers(r.db.WithContext(ctx), userID))
	}

	if serverID != "" {
		query = query.Where("server_id = ?", serverID)
	}

	err := query.Limit(500).Find(&items).Error
	for i := range items {
		items[i].Command = ""
	}

	return items, err
}

// Schedule loads a definition privately, including its command for execution.
func (r *ActivityRepository) Schedule(ctx context.Context, id string) (database.Schedule, error) {
	var item database.Schedule

	err := scheduleUsers(r.db.WithContext(ctx)).Take(&item, "schedules.id = ?", id).Error

	return item, accessError(err)
}

// SaveSchedule replaces a definition; editing explicitly assumes execution ownership.
func (r *ActivityRepository) SaveSchedule(ctx context.Context, item *database.Schedule, create bool) error {
	if create {
		return accessError(r.db.WithContext(ctx).Omit("Server").Create(item).Error)
	}

	result := r.db.WithContext(ctx).Model(&database.Schedule{}).Where("id = ? AND server_id = ?", item.ID, item.ServerID).
		Select("Name", "Action", "Command", "BackupMode", "CronExpression", "TimeZone", "OnlyWhenRunning", "NextRunAt", "Enabled", "RunAsUserID", "AuthorizationScopes").Updates(item)
	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return ErrAccessConflict
	}

	return nil
}

// SetScheduleEnabled changes only scheduling state and execution ownership.
func (r *ActivityRepository) SetScheduleEnabled(ctx context.Context, id, serverID string, enabled bool, userID int64, scopes ...[]string) (database.Schedule, error) {
	var item database.Schedule

	err := database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		if err := scheduleUsers(tx).Take(&item, "schedules.id = ? AND schedules.server_id = ?", id, serverID).Error; err != nil {
			return accessError(err)
		}

		if item.Enabled == enabled {
			return nil
		}

		if enabled {
			if item.CronExpression == "" {
				if item.NextRunAt <= time.Now().Unix() {
					return ErrAccessInput
				}
			} else {
				next, err := recurrence.Next(item.CronExpression, item.TimeZone, time.Now(), 1)
				if err != nil {
					return ErrAccessInput
				}

				item.NextRunAt = next[0]
			}
		}

		item.Enabled, item.RunAsUserID = enabled, userID
		if len(scopes) > 0 {
			item.AuthorizationScopes = scopes[0]
		}

		return tx.Model(&database.Schedule{}).Where("id = ? AND server_id = ?", id, serverID).
			Select("Enabled", "NextRunAt", "RunAsUserID", "AuthorizationScopes").Updates(&item).Error
	})
	if err != nil {
		return database.Schedule{}, err
	}

	return r.Schedule(ctx, id)
}

// DeleteSchedule removes a recurring definition, preserving all execution records.
func (r *ActivityRepository) DeleteSchedule(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&database.Schedule{}, "id = ?", id).Error
}

// Due lists definitions eligible to start; it never creates execution records.
func (r *ActivityRepository) Due(ctx context.Context, now time.Time) ([]database.Schedule, error) {
	return r.DueAfter(ctx, now, nil)
}

// DueAfter pages by due time and identity without consuming pending occurrences.
func (r *ActivityRepository) DueAfter(ctx context.Context, now time.Time, after *database.Schedule) ([]database.Schedule, error) {
	items := []database.Schedule{}

	query := scheduleUsers(r.db.WithContext(ctx)).Where("schedules.enabled = true AND next_run_at <= ?", now.Unix())
	if after != nil {
		query = query.Where("next_run_at > ? OR (next_run_at = ? AND schedules.id > ?)", after.NextRunAt, after.NextRunAt, after.ID)
	}

	err := query.Order("next_run_at, schedules.id").Limit(100).Find(&items).Error

	return items, err
}

// ClaimSchedule snapshots the current definition and atomically consumes its due occurrence.
// A non-nil actor requests an immediate attempt, including for a paused schedule.
func (r *ActivityRepository) ClaimSchedule(ctx context.Context, id string, now time.Time, actor *int64, scopes ...[]string) (database.ScheduleRun, error) {
	var run database.ScheduleRun

	err := database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		var item database.Schedule
		if err := scheduleUsers(tx).Take(&item, "schedules.id = ?", id).Error; err != nil {
			return err
		}

		if actor == nil && (!item.Enabled || item.NextRunAt > now.Unix()) {
			return ErrAccessConflict
		}

		var active int64
		if err := tx.Model(&database.ScheduleRun{}).Where("schedule_id = ? AND state IN ('dispatching','running')", id).Count(&active).Error; err != nil {
			return err
		}

		if active != 0 {
			return ErrAccessConflict
		}

		userID, username := item.RunAsUserID, item.Username

		if actor != nil {
			var user database.User
			if err := tx.Where("disabled = false").Take(&user, *actor).Error; err != nil {
				return err
			}

			userID, username = user.ID, user.Username
		}

		var server database.Server
		if err := tx.Take(&server, "id = ?", item.ServerID).Error; err != nil {
			return err
		}

		run = database.ScheduleRun{ID: rand.Text(), ScheduleID: item.ID, OnlyWhenRunning: item.OnlyWhenRunning, Recurring: actor == nil && item.CronExpression != "", ServerID: item.ServerID, ServerName: server.Name, UserID: userID, Username: username, Action: item.Action, Content: item.Command, State: "dispatching", CreatedAt: now.Unix(), StartedAt: now.Unix()}
		run.AuthorizationScopes = item.AuthorizationScopes

		if item.Action == "backup" {
			run.BackupMode = item.BackupMode.Normalized()
		}

		run.AuthorizationPermission = accessperm.ServerSchedulesManage
		if actor != nil {
			run.AuthorizationPermission = accessperm.ServerSchedulesRun
		}

		if len(scopes) > 0 {
			run.AuthorizationScopes = scopes[0]
		}

		if item.CronExpression == "" {
			if err := tx.Model(&item).Update("enabled", false).Error; err != nil {
				return err
			}
		} else if actor == nil || item.NextRunAt <= now.Unix() {
			next, err := recurrence.Next(item.CronExpression, item.TimeZone, now, 1)
			if err != nil {
				return err
			}

			if err := tx.Model(&item).Update("next_run_at", next[0]).Error; err != nil {
				return err
			}
		}

		return tx.Create(&run).Error
	})

	return run, accessError(err)
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
	return r.runs(ctx, userID, admin, serverID, before, "", false)
}

// ScheduleRuns excludes direct operations while retaining runs of deleted schedules.
func (r *ActivityRepository) ScheduleRuns(ctx context.Context, userID int64, admin bool, serverID, before, scheduleID string) ([]database.ScheduleRun, error) {
	return r.runs(ctx, userID, admin, serverID, before, scheduleID, true)
}

func (r *ActivityRepository) runs(ctx context.Context, userID int64, admin bool, serverID, before, scheduleID string, schedulesOnly bool) ([]database.ScheduleRun, error) {
	items := []database.ScheduleRun{}

	query := r.db.WithContext(ctx).Omit("Content").Order("created_at DESC, id DESC")
	if schedulesOnly {
		query = query.Where("schedule_id <> ''")
	}

	if !admin {
		query = query.Where("server_id IN (?)", visibleServers(r.db.WithContext(ctx), userID))
	}

	if serverID != "" {
		query = query.Where("server_id = ?", serverID)
	}

	if scheduleID != "" {
		query = query.Where("schedule_id = ?", scheduleID)
	}

	query, err := historyBefore(query, &database.ScheduleRun{}, before)
	if err != nil {
		return nil, err
	}

	err = query.Limit(100).Find(&items).Error

	return items, err
}

// Pending returns a bounded batch for the single panel worker.
func (r *ActivityRepository) Pending(ctx context.Context) ([]database.ScheduleRun, error) {
	items := []database.ScheduleRun{}
	err := r.db.WithContext(ctx).Where("state = 'running'").Order("polled_at, created_at, id").Limit(100).Find(&items).Error

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
	case "running", "dispatching":
		updates["started_at"] = now.Unix()
	default:
		updates["finished_at"] = now.Unix()
		updates["content"] = ""
	}

	return r.db.WithContext(ctx).Model(&database.ScheduleRun{}).Where("id = ? AND state IN ('dispatching','running')", id).Updates(updates).Error
}

// FinishRun atomically retains a terminal result and its audit record exactly once.
func (r *ActivityRepository) FinishRun(ctx context.Context, run database.ScheduleRun, state, message string, now time.Time) error {
	return database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		result := tx.Model(&database.ScheduleRun{}).Where("id = ? AND state IN ('dispatching','running')", run.ID).Updates(map[string]any{"state": state, "message": message, "finished_at": now.Unix(), "content": ""})
		if result.Error != nil {
			return result.Error
		}

		if result.RowsAffected == 0 {
			return nil
		}

		var actorID *int64

		var user database.User

		err := tx.Unscoped().Select("id").Take(&user, run.UserID).Error
		if err == nil {
			actorID = &user.ID
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		return tx.Create(&database.AuditEvent{ID: rand.Text(), UserID: actorID, Username: run.Username, Action: "server." + run.Action, Target: "server:" + run.ServerID, Outcome: state, CreatedAt: now.Unix()}).Error
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
		Select("UserID", "Username", "APITokenID", "Action", "Target", "Outcome", "Status").Updates(event).Error
}

// Audit lists administrator-only metadata with bounded pagination and optional filters.
func (r *ActivityRepository) Audit(ctx context.Context, before, action, target string) ([]database.AuditEvent, error) {
	items := []database.AuditEvent{}

	query := r.db.WithContext(ctx).Preload("User", func(tx *gorm.DB) *gorm.DB { return tx.Unscoped() }).Preload("User.Role").Order("created_at DESC, id DESC")

	query, err := historyBefore(query, &database.AuditEvent{}, before)
	if err != nil {
		return nil, err
	}

	if action != "" {
		query = query.Where("action = ?", action)
	}

	if target != "" {
		query = query.Where("target = ?", target)
	}

	err = query.Limit(100).Find(&items).Error

	return items, err
}
