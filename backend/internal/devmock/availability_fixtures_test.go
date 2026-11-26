//go:build !production

package devmock

import (
	"reflect"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
)

func TestAvailabilityFixturesPublishMixedHistoryAndSurviveRestart(t *testing.T) {
	_, db, auth, _ := fixture(t)

	page, targets, err := repository.NewStatusPageRepository(db.ORM()).Published(t.Context(), "mock-status")
	if err != nil || len(targets) != 4 || page != "Mock community status" {
		t.Fatal(page, targets, err)
	}

	ids := []string{"survival", "creative", "modded", "unavailable"}
	store := repository.NewResourceHistoryRepository(db.ORM())
	now := time.Now().Unix()

	before, err := store.Availability(t.Context(), ids, now)
	if err != nil {
		t.Fatal(err)
	}

	for _, id := range ids {
		history := before[id]
		if len(history.Points) != 288 || history.Points[0].OnlineSeconds != 300 {
			t.Fatal(id, history)
		}
	}

	if before["survival"].Points[80].OfflineSeconds != 300 || before["survival"].Points[180].OnlineSeconds != 0 || before["unavailable"].Points[287].OnlineSeconds != 0 || before["modded"].Points[287].OfflineSeconds != 300 {
		t.Fatal("missing mixed availability examples")
	}

	if _, err := New(t.Context(), db.ORM(), auth); err != nil {
		t.Fatal(err)
	}

	after, err := store.Availability(t.Context(), ids, now)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("restart rewrote history", err)
	}

	if err := db.ORM().Delete(&database.StatusPage{}, "id = ?", "mock-availability").Error; err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Where("server_id = ?", "survival").Delete(&database.ServerResourceBucket{}).Error; err != nil {
		t.Fatal(err)
	}

	if _, err := New(t.Context(), db.ORM(), auth); err != nil {
		t.Fatal(err)
	}

	var count int64
	if err := db.ORM().Model(&database.StatusPage{}).Where("id = ?", "mock-availability").Count(&count).Error; err != nil || count != 0 {
		t.Fatal("restart resurrected page", count, err)
	}

	if err := db.ORM().Model(&database.ServerResourceBucket{}).Where("server_id = ?", "survival").Count(&count).Error; err != nil || count != 0 {
		t.Fatal("restart resurrected history", count, err)
	}
}
