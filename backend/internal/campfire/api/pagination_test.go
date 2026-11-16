package api_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/pagination"
	"campfire/internal/shared/model"
	"fmt"
)

func unmarshalPageItems(data []byte, target any) error {
	var page struct {
		Items json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(data, &page); err != nil {
		return err
	}

	return json.Unmarshal(page.Items, target)
}
func emptyPage(data []byte) bool {
	var page pagination.Page[json.RawMessage]
	return json.Unmarshal(data, &page) == nil && page.Items != nil && len(page.Items) == 0 && page.Pagination.TotalItems == 0 && page.Pagination.TotalPages == 0 && page.Pagination.Page == 1
}

func TestSchedulesAreNotSilentlyCappedAndExecutionsPageAuthorizedServers(t *testing.T) {
	f := newAccessFixture(t)
	_, _, input := serverFixture(t, f.admin)
	response := request(f.admin, "POST", "/api/v1/servers", configJSON(t, input))

	var server model.Server
	if response.Code != http.StatusCreated || json.Unmarshal(response.Body.Bytes(), &server) != nil {
		t.Fatal(response.Code, response.Body)
	}

	schedules := make([]database.Schedule, 501)
	for i := range schedules {
		schedules[i] = database.Schedule{ID: fmt.Sprintf("definition-%03d", i), ServerID: server.ID, Name: "Scheduled operation", Action: "restart", Command: "private", RunAsUserID: 1}
	}

	if err := f.db.ORM().Omit("Server").CreateInBatches(schedules, 100).Error; err != nil {
		t.Fatal(err)
	}

	store := repository.NewActivityRepository(f.db.ORM())

	definitions, err := store.Schedules(t.Context(), 1, true, "")
	if err != nil || len(definitions) != 501 {
		t.Fatal("schedule definitions were truncated", len(definitions), err)
	}

	for i := 0; i < 111; i++ {
		serverID := server.ID
		if i < 60 {
			serverID = "inaccessible-server"
		}

		if err := store.CreateRun(t.Context(), &database.ScheduleRun{ID: fmt.Sprintf("run-%03d", i), ServerID: serverID, ScheduleID: schedules[0].ID, Content: "private command", CreatedAt: 1, State: "succeeded"}); err != nil {
			t.Fatal(err)
		}
	}

	first, err := store.ExecutionPage(t.Context(), repository.ExecutionAccess{ServerIDs: []string{server.ID}}, "", "", pagination.Request{Page: 1, Size: 50})
	if err != nil || len(first.Items) != 50 || first.Pagination.TotalItems != 51 || first.Pagination.TotalPages != 2 {
		t.Fatal(first, err)
	}

	second, err := store.ExecutionPage(t.Context(), repository.ExecutionAccess{ServerIDs: []string{server.ID}}, "", "", pagination.Request{Page: 2, Size: 50})
	if err != nil || len(second.Items) != 1 || second.Items[0].Content != "" || second.Items[0].ID != "run-060" {
		t.Fatal(second, err)
	}
}

func TestNotificationNumberedPagesCountOnlyThisInboxAndUnreadFilter(t *testing.T) {
	f := newAccessFixture(t)
	store := repository.NewNotificationRepository(f.db.ORM())

	for i := 0; i < 101; i++ {
		item := database.Notification{TargetType: model.NotificationAccount, TargetID: "1", Kind: model.NotificationTest, Severity: model.NotificationInfo, Title: fmt.Sprintf("Message %03d", i), CreatedAt: 1}
		if err := store.Emit(t.Context(), &item, false); err != nil {
			t.Fatal(err)
		}
	}

	access := repository.NotificationAccess{UserID: 1, Admin: true, ServerView: true}

	first, err := store.Page(t.Context(), access, false, pagination.Request{Page: 1, Size: 50})
	if err != nil || len(first.Items) != 50 || first.Pagination.TotalItems != 101 || first.Pagination.TotalPages != 3 {
		t.Fatal(first, err)
	}

	last, err := store.Page(t.Context(), access, false, pagination.Request{Page: 3, Size: 50})
	if err != nil || len(last.Items) != 1 || last.Items[0].Title != "Message 000" {
		t.Fatal(last, err)
	}

	if err := store.MarkRead(t.Context(), access, first.Items[0].ID, true, time.Now()); err != nil {
		t.Fatal(err)
	}

	unread, err := store.Page(t.Context(), access, true, pagination.Request{Page: 2, Size: 50})
	if err != nil || len(unread.Items) != 50 || unread.Pagination.TotalItems != 100 || unread.Pagination.TotalPages != 2 {
		t.Fatal(unread, err)
	}

	other, err := store.Page(t.Context(), repository.NotificationAccess{UserID: 999}, false, pagination.Request{Page: 1, Size: 50})
	if err != nil || len(other.Items) != 0 || other.Pagination.TotalItems != 0 {
		t.Fatal(other, err)
	}
}

func TestActiveLogAlertsAreNotSilentlyCapped(t *testing.T) {
	f := newAccessFixture(t)
	spark, _, input := serverFixture(t, f.admin)
	response := request(f.admin, "POST", "/api/v1/servers", configJSON(t, input))

	var server model.Server
	if response.Code != http.StatusCreated || json.Unmarshal(response.Body.Bytes(), &server) != nil {
		t.Fatal(response.Code, response.Body)
	}

	records := make([]database.LogAlertIssue, 65)
	for i := range records {
		id := fmt.Sprintf("alert-%03d", i)
		records[i] = database.LogAlertIssue{ID: id, ServerID: server.ID, SparkID: spark.ID, ExecutionID: "run", Phase: model.LogRuntime, DetectedAt: 1, Detection: model.DetectedLogAlert{ID: id, Title: id, Line: "private evidence"}}
	}

	if err := f.db.ORM().Omit("Server").CreateInBatches(records, 100).Error; err != nil {
		t.Fatal(err)
	}

	alerts, err := repository.NewLogAlertRepository(f.db.ORM()).List(t.Context(), server.ID, false)
	if err != nil || len(alerts) != 65 {
		t.Fatal("active alerts were silently truncated", len(alerts), err)
	}

	for _, alert := range alerts {
		if alert.Line != "" {
			t.Fatal("private alert evidence leaked")
		}
	}
}

func TestHistoryPaginationValidation(t *testing.T) {
	f := newAccessFixture(t)
	for _, query := range []string{"page=0", "page=-1", "page=1.5", "page=abc", "page=1&page=2", "page_size=0", "page_size=101", "page_size=abc", "page_size=1&page_size=2", "page=999999999999999999999", "page=2147483647", "before=old", "limit=1"} {
		expectAccess(t, f.admin, "GET", "/api/v1/jobs?"+query, "", http.StatusBadRequest)
	}

	response := request(f.admin, "GET", "/api/v1/jobs?page=3&page_size=10", "")

	var result pagination.Page[json.RawMessage]
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &result) != nil || result.Items == nil || len(result.Items) != 0 || result.Pagination.Page != 3 || result.Pagination.PageSize != 10 || result.Pagination.TotalItems != 0 {
		t.Fatal(response.Code, response.Body)
	}
}
