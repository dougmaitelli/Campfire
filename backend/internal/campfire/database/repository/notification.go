package repository

import (
	"context"
	"errors"
	"strconv"
	"time"

	"campfire/internal/campfire/database"

	"gorm.io/gorm"
)

// ErrNotificationNotFound hides other users' messages and inaccessible targets.
var ErrNotificationNotFound = errors.New("notification not found")

// NotificationRepository persists inboxes, alert transitions and external delivery.
type NotificationRepository struct{ db *gorm.DB }

// NewNotificationRepository uses the panel's database and access assignments.
func NewNotificationRepository(db *gorm.DB) *NotificationRepository {
	return &NotificationRepository{db: db}
}

// NotificationAccess carries effective request permissions, including token scopes.
type NotificationAccess struct {
	UserID                       int64
	Admin, ServerView, SparkView bool
}

// NotificationView includes the requesting user's own read timestamp.
type NotificationView struct {
	database.Notification
	ReadAt int64 `json:"read_at"`
}

func notificationVisible(tx *gorm.DB, access NotificationAccess) *gorm.DB {
	return tx.Model(&database.Notification{}).Joins("JOIN notification_recipients ON notification_recipients.notification_id = notifications.id AND notification_recipients.user_id = ?", access.UserID).
		Where("(notifications.target_type = 'account' AND notifications.target_id = ?) OR (notifications.target_type = 'server' AND ? AND (? OR notifications.target_id IN (?))) OR (notifications.target_type = 'spark' AND ?)", strconv.FormatInt(access.UserID, 10), access.ServerView, access.Admin, visibleServers(tx, access.UserID), access.SparkView).
		Where("notifications.target_type <> 'server' OR ? OR notifications.target_id IN (SELECT server_id FROM server_ownerships WHERE user_id = ?) OR notifications.target_id IN (SELECT server_id FROM server_members JOIN role_permissions ON role_permissions.role_id = server_members.role_id WHERE user_id = ? AND permission = 'server.view')", access.Admin, access.UserID, access.UserID)
}

// List applies live access before pagination and never returns another inbox.
func (r *NotificationRepository) List(ctx context.Context, access NotificationAccess, before int64, unread bool) ([]NotificationView, error) {
	items := make([]NotificationView, 0)

	query := notificationVisible(r.db.WithContext(ctx), access)
	if before > 0 {
		query = query.Where("notifications.id < ?", before)
	}

	if unread {
		query = query.Where("notification_recipients.read_at = 0")
	}

	err := query.Select("notifications.*, notification_recipients.read_at AS read_at").Order("notifications.id DESC").Limit(100).Scan(&items).Error

	return items, err
}

// Unread counts only messages still visible with the user's current permissions.
func (r *NotificationRepository) Unread(ctx context.Context, access NotificationAccess) (int64, error) {
	var count int64

	err := notificationVisible(r.db.WithContext(ctx), access).Where("notification_recipients.read_at = 0").Count(&count).Error

	return count, err
}

// MarkRead changes only the caller's receipt; revoked targets remain inaccessible.
func (r *NotificationRepository) MarkRead(ctx context.Context, access NotificationAccess, id int64, read bool, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := notificationVisible(tx, access).Where("notifications.id = ?", id).Count(&count).Error; err != nil {
			return err
		}

		if count == 0 {
			return ErrNotificationNotFound
		}

		stamp := int64(0)
		if read {
			stamp = now.Unix()
		}

		return tx.Model(&database.NotificationRecipient{}).Where("notification_id = ? AND user_id = ?", id, access.UserID).Update("read_at", stamp).Error
	})
}

// MarkAllRead uses an upper bound so newly arriving messages are not acknowledged.
func (r *NotificationRepository) MarkAllRead(ctx context.Context, access NotificationAccess, through int64, now time.Time) error {
	query := notificationVisible(r.db.WithContext(ctx), access)
	if through > 0 {
		query = query.Where("notifications.id <= ?", through)
	}

	return r.db.WithContext(ctx).Model(&database.NotificationRecipient{}).Where("user_id = ? AND read_at = 0 AND notification_id IN (?)", access.UserID, query.Select("notifications.id")).Update("read_at", now.Unix()).Error
}

func emitNotification(tx *gorm.DB, item *database.Notification, queue bool) error {
	if err := tx.Create(item).Error; err != nil {
		return err
	}

	users := tx.Model(&database.User{}).Where("disabled = false")

	switch item.TargetType {
	case "account":
		users = users.Where("id = ?", item.TargetID)
	case "spark":
		users = users.Where("role_id = ?", database.AdminRole)
	case "server":
		users = users.Where("role_id = ? OR id IN (SELECT user_id FROM server_ownerships WHERE server_id = ?) OR id IN (SELECT server_members.user_id FROM server_members JOIN role_permissions ON role_permissions.role_id = server_members.role_id WHERE server_id = ? AND permission = 'server.view')", database.AdminRole, item.TargetID, item.TargetID)
	default:
		return ErrAccessInput
	}

	var ids []int64
	if err := users.Pluck("id", &ids).Error; err != nil {
		return err
	}

	recipients := make([]database.NotificationRecipient, 0, len(ids))
	for _, id := range ids {
		recipients = append(recipients, database.NotificationRecipient{NotificationID: item.ID, UserID: id})
	}

	if len(recipients) > 0 {
		if err := tx.Omit("Notification", "User").CreateInBatches(recipients, 100).Error; err != nil {
			return err
		}
	}

	if queue {
		return tx.Omit("Notification").Create(&database.NotificationDelivery{NotificationID: item.ID, State: "pending", NextAttemptAt: item.CreatedAt}).Error
	}

	return nil
}

// Emit atomically stores a message, recipients and optional external delivery.
func (r *NotificationRepository) Emit(ctx context.Context, item *database.Notification, queue bool) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return emitNotification(tx, item, queue) })
}

// Observe commits an alert state transition and its notification together.
func (r *NotificationRepository) Observe(ctx context.Context, key string, queue bool, transition func(*database.AlertState) *database.Notification) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var state database.AlertState
		if err := tx.Take(&state, "key = ?", key).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		state.Key = key

		item := transition(&state)
		if err := tx.Save(&state).Error; err != nil {
			return err
		}

		if item != nil {
			return emitNotification(tx, item, queue)
		}

		return nil
	})
}

// ClearAlert forgets a deliberately disabled condition without claiming recovery.
func (r *NotificationRepository) ClearAlert(ctx context.Context, key string) error {
	return r.db.WithContext(ctx).Delete(&database.AlertState{}, "key = ?", key).Error
}

// PruneAlertTargets removes state for targets that were deleted or disabled.
func (r *NotificationRepository) PruneAlertTargets(ctx context.Context) error {
	if err := r.db.WithContext(ctx).Where("target_type = 'spark' AND target_id NOT IN (SELECT id FROM sparks WHERE disabled = false)").Delete(&database.AlertState{}).Error; err != nil {
		return err
	}

	return r.db.WithContext(ctx).Where("target_type = 'server' AND target_id NOT IN (SELECT servers.id FROM servers JOIN sparks ON sparks.id = servers.spark_id WHERE sparks.disabled = false)").Delete(&database.AlertState{}).Error
}

// DueDeliveries loads a bounded batch; only the single panel delivery worker sends it.
func (r *NotificationRepository) DueDeliveries(ctx context.Context, now time.Time) ([]database.NotificationDelivery, error) {
	items := make([]database.NotificationDelivery, 0)
	err := r.db.WithContext(ctx).Preload("Notification").Where("state = 'pending' AND next_attempt_at <= ?", now.Unix()).Order("next_attempt_at, notification_id").Limit(10).Find(&items).Error

	return items, err
}

// FinishDelivery records a bounded retry policy separately from the inbox.
func (r *NotificationRepository) FinishDelivery(ctx context.Context, item database.NotificationDelivery, message string, now time.Time) error {
	item.Attempts++

	item.Error = message
	if message == "" {
		item.State = "delivered"
		item.DeliveredAt = now.Unix()
	} else if item.Attempts >= 5 {
		item.State = "failed"
	} else {
		item.NextAttemptAt = now.Add(time.Duration(30*(1<<(item.Attempts-1))) * time.Second).Unix()
	}

	return r.db.WithContext(ctx).Model(&database.NotificationDelivery{}).Where("notification_id = ? AND state = 'pending'", item.NotificationID).Select("State", "Attempts", "NextAttemptAt", "DeliveredAt", "Error").Updates(item).Error
}

// DeliveryCounts exposes only aggregate status, never endpoint credentials.
func (r *NotificationRepository) DeliveryCounts(ctx context.Context) (int64, int64, error) {
	var pending, failed int64
	if err := r.db.WithContext(ctx).Model(&database.NotificationDelivery{}).Where("state = 'pending'").Count(&pending).Error; err != nil {
		return 0, 0, err
	}

	err := r.db.WithContext(ctx).Model(&database.NotificationDelivery{}).Where("state = 'failed'").Count(&failed).Error

	return pending, failed, err
}

// RetryDeliveries explicitly requeues exhausted external deliveries.
func (r *NotificationRepository) RetryDeliveries(ctx context.Context, now time.Time) error {
	return r.db.WithContext(ctx).Model(&database.NotificationDelivery{}).Where("state = 'failed'").Updates(map[string]any{"state": "pending", "attempts": 0, "error": "", "next_attempt_at": now.Unix()}).Error
}
