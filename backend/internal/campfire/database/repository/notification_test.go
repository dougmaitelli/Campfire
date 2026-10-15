package repository_test

import (
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
)

func TestNotificationReadWatermarksAndDeliveryExhaustion(t *testing.T) {
	db, _ := historyStore(t)
	if err := db.ORM().Create(&database.User{ID: 1, Username: "admin", RoleID: database.AdminRole, Version: 1}).Error; err != nil {
		t.Fatal(err)
	}

	store := repository.NewNotificationRepository(db.ORM())
	access := repository.NotificationAccess{UserID: 1, Admin: true, ServerView: true, SparkView: true}
	ctx := t.Context()

	first := database.Notification{TargetType: "account", TargetID: "1", Title: "First", CreatedAt: time.Now().Unix()}
	if err := store.Emit(ctx, &first, true); err != nil {
		t.Fatal(err)
	}

	second := database.Notification{TargetType: "account", TargetID: "1", Title: "Second", CreatedAt: time.Now().Unix()}
	if err := store.Emit(ctx, &second, false); err != nil {
		t.Fatal(err)
	}

	if err := store.MarkAllRead(ctx, access, first.ID, time.Now()); err != nil {
		t.Fatal(err)
	}

	count, err := store.Unread(ctx, access)
	if err != nil || count != 1 {
		t.Fatal("read-all acknowledged a newer message", count, err)
	}

	items, err := store.List(ctx, access, second.ID, false)
	if err != nil || len(items) != 1 || items[0].ID != first.ID {
		t.Fatal("notification cursor failed", len(items), err)
	}

	now := time.Now()
	for attempt := range 5 {
		due, err := store.DueDeliveries(ctx, now)
		if err != nil || len(due) != 1 {
			t.Fatal("queued delivery missing", attempt, err)
		}

		if err := store.FinishDelivery(ctx, due[0], "Apprise request failed", now); err != nil {
			t.Fatal(err)
		}

		now = now.Add(time.Hour)
	}

	pending, failed, err := store.DeliveryCounts(ctx)
	if err != nil || pending != 0 || failed != 1 {
		t.Fatal("unbounded external retries", pending, failed, err)
	}

	if err := store.RetryDeliveries(ctx, now); err != nil {
		t.Fatal(err)
	}

	due, err := store.DueDeliveries(ctx, now)
	if err != nil || len(due) != 1 || due[0].Attempts != 0 {
		t.Fatal("manual retry did not reset exhausted delivery", err)
	}

	if err := db.ORM().Delete(&database.User{}, 1).Error; err != nil {
		t.Fatal(err)
	}

	var receipts int64
	if err := db.ORM().Model(&database.NotificationRecipient{}).Count(&receipts).Error; err != nil || receipts != 0 {
		t.Fatal("user deletion left read receipts", receipts, err)
	}

	if err := db.ORM().Delete(&database.Notification{}, first.ID).Error; err != nil {
		t.Fatal(err)
	}

	var deliveries int64
	if err := db.ORM().Model(&database.NotificationDelivery{}).Count(&deliveries).Error; err != nil || deliveries != 0 {
		t.Fatal("message deletion left delivery data", deliveries, err)
	}
}
