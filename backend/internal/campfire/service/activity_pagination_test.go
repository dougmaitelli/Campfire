package service

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"campfire/internal/campfire/database"
)

type pagedScheduleConnectivity struct{ reconnected atomic.Bool }

func (c *pagedScheduleConnectivity) Connected(id string) bool {
	return id != "offline" || c.reconnected.Load()
}

func TestOfflineSchedulePageDoesNotStarveOnlineWork(t *testing.T) {
	s, db, _, owner := activityFixture(t)
	online := savedSchedule(t, s, owner, "command")
	connection := &pagedScheduleConnectivity{}
	s.deployment.Sparks.connectivity = connection

	node := database.Spark{ID: "offline", Name: "Offline", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if err := db.ORM().Create(&node).Error; err != nil {
		t.Fatal(err)
	}

	var server database.Server

	if err := db.ORM().Take(&server, "id = ?", "game").Error; err != nil {
		t.Fatal(err)
	}

	server.ID, server.Name, server.SparkID = "offline-game", "Offline game", node.ID
	if err := db.ORM().Omit("Spark", "Ember").Create(&server).Error; err != nil {
		t.Fatal(err)
	}

	old := online.NextRunAt - 3600

	items := make([]database.Schedule, 100)
	for i := range items {
		items[i] = database.Schedule{ID: fmt.Sprintf("offline-%03d", i), ServerID: server.ID, Name: "Offline", Action: "command", Command: "pending", TimeZone: "UTC", NextRunAt: old, Enabled: true, RunAsUserID: owner.UserID}
	}

	if err := db.ORM().Omit("Server").Create(&items).Error; err != nil {
		t.Fatal(err)
	}

	first, err := s.Store.Due(t.Context(), time.Now())
	if err != nil || len(first) != 100 || first[99].ID != "offline-099" {
		t.Fatal("fixture did not fill the oldest page", first, err)
	}

	next, err := s.Store.DueAfter(t.Context(), time.Now(), &first[99])
	if err != nil || len(next) != 1 || next[0].ID != online.ID {
		t.Fatal("pagination missed the later online schedule", next, err)
	}

	if err := s.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	waitForRun := func(serverID string) {
		t.Helper()

		deadline := time.Now().Add(8 * time.Second)

		for {
			var count int64
			if err := db.ORM().Model(&database.ScheduleRun{}).Where("server_id = ? AND state = 'succeeded'", serverID).Count(&count).Error; err != nil {
				t.Fatal(err)
			}

			if count > 0 {
				return
			}

			if time.Now().After(deadline) {
				t.Fatal("schedule never executed", serverID)
			}

			time.Sleep(10 * time.Millisecond)
		}
	}

	waitForRun("game")

	var pending, runs int64

	if err := db.ORM().Model(&database.Schedule{}).Where("server_id = ? AND enabled = true AND next_run_at = ?", server.ID, old).Count(&pending).Error; err != nil || pending != 100 {
		t.Fatal("offline occurrences were consumed or postponed", pending, err)
	}

	if err := db.ORM().Model(&database.ScheduleRun{}).Where("server_id = ?", server.ID).Count(&runs).Error; err != nil || runs != 0 {
		t.Fatal("offline schedules created execution records", runs, err)
	}

	connection.reconnected.Store(true)
	waitForRun(server.ID)
}
