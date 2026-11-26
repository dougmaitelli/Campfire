//go:build !production

package devmock

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/pagination"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

func TestHistoryExamplesProvideMultipleFilteredPages(t *testing.T) {
	runtime, db, _, _ := fixture(t)
	if err := runtime.EnableJobExamples(t.Context()); err != nil {
		t.Fatal(err)
	}

	if err := runtime.EnableHistoryExamples(t.Context()); err != nil {
		t.Fatal(err)
	}

	request := pagination.Request{Page: 2, Size: 50}
	check := func(items int, metadata pagination.Metadata) {
		t.Helper()

		if items != 50 || metadata.Page != 2 || metadata.PageSize != 50 || metadata.TotalItems < historyExampleCount || metadata.TotalPages < 3 {
			t.Fatal(items, metadata)
		}
	}
	jobs := repository.NewJobRepository(db.ORM())

	jobPage, err := jobs.Page(t.Context(), "", nil, repository.JobFilter{}, request)
	if err != nil {
		t.Fatal(err)
	}

	check(len(jobPage.Items), jobPage.Pagination)

	completed, err := jobs.Page(t.Context(), "", nil, repository.JobFilter{Type: model.JobCopy, Status: model.JobCompleted}, request)
	if err != nil || len(completed.Items) == 0 || completed.Pagination.TotalPages < 2 {
		t.Fatal("missing filtered job pages", completed, err)
	}

	inbox := repository.NewNotificationRepository(db.ORM())
	access := repository.NotificationAccess{UserID: 1, Admin: true, ServerView: true, SparkView: true}

	notes, err := inbox.Page(t.Context(), access, false, request)
	if err != nil {
		t.Fatal(err)
	}

	check(len(notes.Items), notes.Pagination)

	unread, err := inbox.Page(t.Context(), access, true, request)
	if err != nil || len(unread.Items) == 0 || unread.Pagination.TotalPages < 2 {
		t.Fatal("missing unread pages", unread, err)
	}

	for _, note := range unread.Items {
		if note.ReadAt != 0 {
			t.Fatal("unread filter returned read notification", note.ID)
		}
	}

	history := repository.NewActivityRepository(db.ORM())

	runs, err := history.ExecutionPage(t.Context(), repository.ExecutionAccess{AllServers: true}, "", "", request)
	if err != nil {
		t.Fatal(err)
	}

	check(len(runs.Items), runs.Pagination)

	nightly, err := history.ExecutionPage(t.Context(), repository.ExecutionAccess{ServerIDs: []string{"survival"}}, "", "nightly-restart", request)
	if err != nil || len(nightly.Items) < 15 || nightly.Pagination.TotalPages < 2 {
		t.Fatal("nightly drawer has no second page", nightly, err)
	}

	retired, err := history.ExecutionPage(t.Context(), repository.ExecutionAccess{AllServers: true}, "", historyRetiredScheduleID, pagination.Request{Page: 1, Size: 50})
	if err != nil || len(retired.Items) != 10 {
		t.Fatal("missing deleted-schedule history", retired, err)
	}

	archived, err := history.ExecutionPage(t.Context(), repository.ExecutionAccess{AllServers: true}, historyArchivedServerID, "", pagination.Request{Page: 1, Size: 50})
	if err != nil || len(archived.Items) != 10 || archived.Items[0].ServerName == "" {
		t.Fatal("missing deleted-server history", archived, err)
	}

	denied, err := history.ExecutionPage(t.Context(), repository.ExecutionAccess{ServerIDs: []string{"survival"}}, historyArchivedServerID, "", request)
	if err != nil || denied.Pagination.TotalItems != 0 {
		t.Fatal("retained example bypasses server access", denied, err)
	}

	audit, err := history.AuditPage(t.Context(), "", "", request)
	if err != nil {
		t.Fatal(err)
	}

	check(len(audit.Items), audit.Pagination)

	filtered, err := history.AuditPage(t.Context(), "server.start", "server:survival", request)
	if err != nil || len(filtered.Items) == 0 || filtered.Pagination.TotalPages < 2 {
		t.Fatal("missing filtered audit pages", filtered, err)
	}

	activity := repository.NewServerActivityRepository(db.ORM())
	activityAccess := repository.ServerActivityAccess{Console: true, ScheduledOperations: true, ManualOperations: true, JobTypes: []model.JobType{model.JobCopy, model.JobTransfer}}

	entries, err := activity.List(t.Context(), "survival", activityAccess, repository.ServerActivityFilter{}, request)
	if err != nil {
		t.Fatal(err)
	}

	check(len(entries.Items), *entries.Pagination)

	mixed, err := activity.List(t.Context(), "survival", activityAccess, repository.ServerActivityFilter{}, pagination.Request{Page: 1, Size: 100})
	if err != nil {
		t.Fatal(err)
	}

	kinds := map[string]bool{}
	for _, entry := range mixed.Items {
		kinds[entry.Kind] = true
	}

	if len(kinds) < 3 {
		t.Fatal("activity fixture lacks mixed sources", kinds)
	}

	game, err := activity.List(t.Context(), "survival", activityAccess, repository.ServerActivityFilter{Kind: "game", GameType: "player.joined"}, request)
	if err != nil || len(game.Items) == 0 || game.Pagination.TotalPages < 2 {
		t.Fatal("missing filtered game pages", game, err)
	}
}

func TestHistoryExamplesPreserveEditsAndNeverQueueWork(t *testing.T) {
	runtime, db, auth, servers := fixture(t)
	now := time.Now().UTC()

	runtime.now = func() time.Time { return now }
	if err := runtime.EnableHistoryExamples(t.Context()); err != nil {
		t.Fatal(err)
	}

	var jobs []database.ServerJob
	if err := db.ORM().Where("id LIKE ?", historyExamplePrefix+"job-%").Find(&jobs).Error; err != nil || len(jobs) != historyExampleCount {
		t.Fatal(len(jobs), err)
	}

	for _, job := range jobs {
		if job.Active() || job.FinishedAt == nil {
			t.Fatal("bulk history reserved a server", job.ID)
		}
	}

	for _, record := range []any{&database.NotificationDelivery{}, &database.IntegrationMessage{}} {
		var count int64
		if err := db.ORM().Model(record).Count(&count).Error; err != nil || count != 0 {
			t.Fatal("historical seed queued delivery work", fmt.Sprintf("%T", record), count, err)
		}
	}

	var note database.Notification
	if err := db.ORM().Where("title = ?", "Pagination message 002").Take(&note).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Model(&database.NotificationRecipient{}).Where("notification_id = ? AND user_id = ?", note.ID, 1).Update("read_at", now.Unix()).Error; err != nil {
		t.Fatal(err)
	}

	jobID := historyExamplePrefix + "job-001"
	if err := db.ORM().Delete(&database.ServerJob{}, "id = ?", jobID).Error; err != nil {
		t.Fatal(err)
	}

	restarted, err := New(t.Context(), db.ORM(), auth)
	if err != nil {
		t.Fatal(err)
	}

	if err := restarted.EnableHistoryExamples(t.Context()); err != nil {
		t.Fatal(err)
	}

	for _, row := range []struct {
		model  any
		filter string
		value  any
		count  int64
	}{
		{&database.ServerJob{}, "id LIKE ?", historyExamplePrefix + "job-%", historyExampleCount - 1},
		{&database.ScheduleRun{}, "id LIKE ?", historyExamplePrefix + "execution-%", historyExampleCount},
		{&database.AuditEvent{}, "id LIKE ?", historyExamplePrefix + "audit-%", historyExampleCount},
		{&database.GameEvent{}, "event_id LIKE ?", historyExamplePrefix + "event-%", historyExampleCount},
		{&database.Notification{}, "title LIKE ?", "Pagination message %", historyExampleCount},
	} {
		var count int64
		if err := db.ORM().Model(row.model).Where(row.filter, row.value).Count(&count).Error; err != nil || count != row.count {
			t.Fatal("restart duplicated or reset history", fmt.Sprintf("%T", row.model), count, err)
		}
	}

	var recipient database.NotificationRecipient
	if err := db.ORM().Where("notification_id = ? AND user_id = ?", note.ID, 1).Take(&recipient).Error; err != nil || recipient.ReadAt != now.Unix() {
		t.Fatal("restart lost read state", recipient, err)
	}

	var fleet int64
	if err := db.ORM().Model(&database.Server{}).Count(&fleet).Error; err != nil || fleet != int64(len(servers)) {
		t.Fatal("history seed created or recreated servers", fleet, err)
	}
}

func TestHistoryExamplesRollBackPartialSeeds(t *testing.T) {
	runtime, db, _, _ := fixture(t)
	failure := errors.New("reject historical event fixture")

	const callback = "test:history-seed-failure"
	if err := db.ORM().Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "game_events" {
			_ = tx.AddError(failure)
		}
	}); err != nil {
		t.Fatal(err)
	}

	if err := runtime.EnableHistoryExamples(t.Context()); !errors.Is(err, failure) {
		t.Fatal("expected a failed seed", err)
	}

	for _, row := range []struct {
		model  any
		filter string
		value  any
	}{
		{&database.ServerJob{}, "id LIKE ?", historyExamplePrefix + "job-%"},
		{&database.ScheduleRun{}, "id LIKE ?", historyExamplePrefix + "execution-%"},
		{&database.Notification{}, "title LIKE ?", "Pagination message %"},
		{&seedMarker{}, "id = ?", historySeedMarkerID},
	} {
		var count int64
		if err := db.ORM().Model(row.model).Where(row.filter, row.value).Count(&count).Error; err != nil || count != 0 {
			t.Fatal("partial history seed survived rollback", fmt.Sprintf("%T", row.model), count, err)
		}
	}

	if err := db.ORM().Callback().Create().Remove(callback); err != nil {
		t.Fatal(err)
	}

	if err := runtime.EnableHistoryExamples(t.Context()); err != nil {
		t.Fatal("failed seed could not be retried", err)
	}
}
