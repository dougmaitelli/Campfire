package database

import "gorm.io/gorm"

// backfillActivitySources restores only references already encoded exactly in
// historical event IDs. Notification text and timestamps are never used to guess.
func backfillActivitySources(tx *gorm.DB) error {
	return tx.Exec("UPDATE integration_messages SET source_key = event_id WHERE source_key = '' AND ((category = 'console_alert' AND substr(event_id, 1, 14) = 'console_alert:') OR (category IN ('power', 'backup') AND substr(event_id, 1, 10) = 'operation:'))").Error
}

// Historical issues use an exact source-linked panel receipt when retained.
// Older detections without that evidence keep their original timestamp.
func backfillAlertReceipts(tx *gorm.DB) error {
	return tx.Exec("UPDATE log_alert_issues SET received_at = COALESCE((SELECT MIN(created_at) FROM notifications WHERE target_type = 'server' AND target_id = log_alert_issues.server_id AND source_key = 'console_alert:' || log_alert_issues.id), detected_at) WHERE received_at = 0").Error
}
