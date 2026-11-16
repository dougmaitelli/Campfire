package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"campfire/internal/campfire/api"
	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/pagination"
	"campfire/internal/campfire/service"

	"gorm.io/gorm"
)

func TestExecutionPageBatchesServerPermissions(t *testing.T) {
	f := newAccessFixture(t)
	spark, ember, _ := serverFixture(t, f.admin)
	servers := service.NewServerService(repository.NewServerRepository(f.db.ORM()), nil)
	store := repository.NewActivityRepository(f.db.ORM())
	panel := api.New(nil, nil, servers).WithActivity(service.NewActivityService(store, nil, f.auth))
	f.admin = authenticatedTestAPI(t, f.db, panel)
	f.panel = panel.Handler()
	member, regular := f.user("schedule-reader", database.UserRole)

	addServer := func(index int) {
		t.Helper()

		owner := int64(1)
		id := fmt.Sprintf("history-server-%02d", index)

		row := database.Server{ID: id, Name: id, SparkID: spark.ID, EmberID: ember.ID, OwnerUserID: &owner, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
		if err := f.db.ORM().Omit("Spark", "Ember", "Owner").Create(&row).Error; err != nil {
			t.Fatal(err)
		}

		grant := database.ServerAccessGrant{ServerID: id, UserID: &member.ID, AccessRoleID: database.OperatorRole}
		if err := f.db.ORM().Omit("User", "Role", "AccessRole").Create(&grant).Error; err != nil {
			t.Fatal(err)
		}

		if err := store.CreateRun(t.Context(), &database.ScheduleRun{ID: id, ServerID: id, ScheduleID: "schedule", State: "succeeded", CreatedAt: 1}); err != nil {
			t.Fatal(err)
		}
	}
	addServer(0)

	var reads atomic.Int64

	count := func(*gorm.DB) { reads.Add(1) }
	if err := f.db.ORM().Callback().Query().After("gorm:query").Register("test:execution-reads", count); err != nil {
		t.Fatal(err)
	}

	if err := f.db.ORM().Callback().Row().After("gorm:row").Register("test:execution-reads", count); err != nil {
		t.Fatal(err)
	}

	readPage := func(handler http.Handler, total int64) int64 {
		t.Helper()
		reads.Store(0)

		response := request(handler, "GET", "/api/v1/schedules/executions?page_size=1", "")

		var page pagination.Page[database.ScheduleRun]
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &page) != nil || len(page.Items) != 1 || page.Pagination.TotalItems != total {
			t.Fatal(response.Code, response.Body)
		}

		return reads.Load()
	}
	adminReads := readPage(f.admin, 1)
	memberReads := readPage(regular, 1)

	for i := 1; i < 25; i++ {
		addServer(i)
	}

	if got := readPage(f.admin, 25); got != adminReads {
		t.Fatalf("admin history queries grew from %d to %d", adminReads, got)
	}

	if got := readPage(regular, 25); got != memberReads {
		t.Fatalf("member history queries grew from %d to %d", memberReads, got)
	}
}
