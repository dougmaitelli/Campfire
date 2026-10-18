package database

import (
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

// importPrereleaseSchema preserves pre-release installations while consolidating
// their tables and migration history into the release baseline.
func importPrereleaseSchema(tx *gorm.DB) error {
	m := tx.Migrator()
	if m.HasColumn("users", "version") {
		if err := m.RenameColumn("users", "version", "credential_version"); err != nil {
			return err
		}
	}

	if err := tx.AutoMigrate(&User{}); err != nil {
		return err
	}

	if m.HasColumn("servers", "ember_revision") {
		if err := m.RenameColumn("servers", "ember_revision", "ember_hash"); err != nil {
			return err
		}
	}

	for _, field := range []string{"EnrollmentTokenHash", "EnrollmentExpiresAt"} {
		if !m.HasColumn(&Spark{}, field) {
			if err := m.AddColumn(&Spark{}, field); err != nil {
				return err
			}
		}
	}

	for _, field := range []string{"OwnerUserID", "EmberHash"} {
		if !m.HasColumn(&Server{}, field) {
			if err := m.AddColumn(&Server{}, field); err != nil {
				return err
			}
		}
	}

	for _, field := range []string{"UpdateCheckedAt", "UpdateAvailable", "UpdateError"} {
		if !m.HasColumn(&Ember{}, field) {
			if err := m.AddColumn(&Ember{}, field); err != nil {
				return err
			}
		}
	}

	if err := tx.AutoMigrate(&ServerAccessGrant{}, &SparkAccessGrant{}); err != nil {
		return err
	}

	if err := m.AlterColumn(&AuditEvent{}, "UserID"); err != nil {
		return err
	}

	if err := tx.Exec("UPDATE audit_events SET user_id = NULL WHERE user_id = 0 OR user_id NOT IN (SELECT id FROM users)").Error; err != nil {
		return err
	}

	copies := []struct{ table, sql string }{
		{"server_ownerships", "UPDATE servers SET owner_user_id = (SELECT user_id FROM server_ownerships WHERE server_ownerships.server_id = servers.id)"},
		{"spark_enrollments", "UPDATE sparks SET enrollment_token_hash = (SELECT hash FROM spark_enrollments WHERE spark_enrollments.spark_id = sparks.id), enrollment_expires_at = COALESCE((SELECT expires_at FROM spark_enrollments WHERE spark_enrollments.spark_id = sparks.id), 0)"},
		{"server_members", "INSERT INTO server_access_grants (server_id, user_id, access_role_id) SELECT server_id, user_id, role_id FROM server_members"},
		{"spark_roles", "INSERT INTO spark_access_grants (spark_id, role_id, access_role_id) SELECT spark_id, role_id, 2 FROM spark_roles"},
		{"ember_updates", "UPDATE embers SET update_checked_at = COALESCE((SELECT checked_at FROM ember_updates WHERE ember_updates.ember_id = embers.id), 0), update_available = COALESCE((SELECT available FROM ember_updates WHERE ember_updates.ember_id = embers.id), false), update_error = COALESCE((SELECT error FROM ember_updates WHERE ember_updates.ember_id = embers.id), '')"},
	}
	for _, item := range copies {
		if m.HasTable(item.table) {
			if err := tx.Exec(item.sql).Error; err != nil {
				return err
			}

			if err := m.DropTable(item.table); err != nil {
				return err
			}
		}
	}

	var embers []Ember
	if err := tx.Find(&embers).Error; err != nil {
		return err
	}

	for _, ember := range embers {
		if err := tx.Model(&Server{}).Where("ember_id = ? AND ember_hash = ''", ember.ID).Update("ember_hash", model.EmberHash(ember.Definition)).Error; err != nil {
			return err
		}
	}

	if m.HasConstraint(&Spark{}, "sparks_kind") {
		if err := m.DropConstraint(&Spark{}, "sparks_kind"); err != nil {
			return err
		}
	}

	for _, item := range []struct {
		model  any
		column string
	}{{&Spark{}, "kind"}, {&Ember{}, "update_checked_url"}, {&Schedule{}, "username"}, {&Schedule{}, "version"}, {&ScheduleRun{}, "schedule_version"}} {
		if m.HasColumn(item.model, item.column) {
			if err := m.DropColumn(item.model, item.column); err != nil {
				return err
			}
		}
	}

	if m.HasColumn(&Schedule{}, "user_id") {
		if err := m.RenameColumn(&Schedule{}, "user_id", "run_as_user_id"); err != nil {
			return err
		}
	}
	// Never-started work becomes a pending definition, not execution history.
	queries := []string{
		"INSERT INTO schedules (id,server_id,name,action,command,cron_expression,time_zone,only_when_running,next_run_at,enabled,run_as_user_id,created_at) SELECT 'pending-' || schedule_runs.id,schedule_runs.server_id,'Pending ' || schedule_runs.action,COALESCE(origin.action,schedule_runs.action),COALESCE(origin.command,schedule_runs.content),'','UTC',COALESCE(origin.only_when_running,schedule_runs.only_when_running),schedule_runs.created_at,true,schedule_runs.user_id,schedule_runs.created_at FROM schedule_runs JOIN servers ON servers.id = schedule_runs.server_id JOIN users ON users.id = schedule_runs.user_id LEFT JOIN schedules origin ON origin.id = schedule_runs.schedule_id WHERE schedule_runs.state = 'queued' AND schedule_runs.recurring = false AND users.disabled = false AND users.deleted_at IS NULL",
		"UPDATE schedules SET next_run_at = COALESCE((SELECT MIN(created_at) FROM schedule_runs WHERE schedule_runs.schedule_id = schedules.id AND state = 'queued' AND recurring = true), next_run_at) WHERE enabled = true",
		"DELETE FROM schedule_runs WHERE state = 'queued'",
	}
	for _, query := range queries {
		if err := tx.Exec(query).Error; err != nil {
			return err
		}
	}

	return nil
}
