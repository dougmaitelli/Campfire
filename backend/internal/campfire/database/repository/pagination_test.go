package repository_test

import (
	"fmt"
	"testing"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/pagination"
	"campfire/internal/shared/model"
)

func TestNumberedActivityPagesHaveMatchingFilteredTotals(t *testing.T) {
	db, _ := integrationEventStore(t)
	store := repository.NewServerActivityRepository(db.ORM())
	access := repository.ServerActivityAccess{}

	for i := 0; i < 65; i++ {
		target := "first"
		if i >= 55 {
			target = "other"
		}

		if err := db.ORM().Create(&database.Notification{TargetType: model.NotificationServer, TargetID: target, Title: fmt.Sprintf("Notice %03d", i), CreatedAt: 100}).Error; err != nil {
			t.Fatal(err)
		}
	}

	first, err := store.List(t.Context(), "first", access, repository.ServerActivityFilter{}, pagination.Request{Page: 1, Size: 50})
	if err != nil || len(first.Items) != 50 || first.Pagination.TotalItems != 55 || first.Pagination.TotalPages != 2 {
		t.Fatal(first, err)
	}

	second, err := store.List(t.Context(), "first", access, repository.ServerActivityFilter{}, pagination.Request{Page: 2, Size: 50})
	if err != nil || len(second.Items) != 5 || second.Pagination.Page != 2 {
		t.Fatal(second, err)
	}

	seen := map[string]bool{}
	for _, item := range append(first.Items, second.Items...) {
		if seen[item.Key] {
			t.Fatal("duplicate", item.Key)
		}

		seen[item.Key] = true
	}

	filtered, err := store.List(t.Context(), "first", access, repository.ServerActivityFilter{Search: "Notice 05"}, pagination.Request{Page: 2, Size: 2})
	if err != nil || len(filtered.Items) != 2 || filtered.Pagination.TotalItems != 5 || filtered.Pagination.TotalPages != 3 {
		t.Fatal(filtered, err)
	}
}

func TestNumberedExecutionAndAuditPages(t *testing.T) {
	db, store := historyStore(t)

	for i := 0; i < 55; i++ {
		id := fmt.Sprintf("row-%03d", i)
		if err := store.CreateRun(t.Context(), &database.ScheduleRun{ID: id, ServerID: "visible", ScheduleID: "deleted", CreatedAt: 1, Content: "private"}); err != nil {
			t.Fatal(err)
		}

		if err := db.ORM().Create(&database.AuditEvent{ID: id, Action: "server.start", CreatedAt: 1}).Error; err != nil {
			t.Fatal(err)
		}
	}

	runs, err := store.ExecutionPage(t.Context(), repository.ExecutionAccess{ServerIDs: []string{"visible"}}, "", "deleted", pagination.Request{Page: 2, Size: 50})
	if err != nil || len(runs.Items) != 5 || runs.Pagination.TotalItems != 55 || runs.Items[0].Content != "" {
		t.Fatal(runs, err)
	}

	for _, scope := range []repository.ExecutionAccess{{}, {ServerIDs: []string{}}, {ServerIDs: []string{"other"}}} {
		empty, err := store.ExecutionPage(t.Context(), scope, "visible", "", pagination.Request{Page: 1, Size: 50})
		if err != nil || len(empty.Items) != 0 || empty.Pagination.TotalItems != 0 {
			t.Fatal("empty or unrelated allowlist granted history access", empty, err)
		}
	}

	all, err := store.ExecutionPage(t.Context(), repository.ExecutionAccess{AllServers: true}, "", "deleted", pagination.Request{Page: 2, Size: 50})
	if err != nil || len(all.Items) != 5 || all.Pagination.TotalItems != 55 {
		t.Fatal("unrestricted history omitted retained runs", all, err)
	}

	audit, err := store.AuditPage(t.Context(), "server.start", "", pagination.Request{Page: 2, Size: 50})
	if err != nil || len(audit.Items) != 5 || audit.Pagination.TotalItems != 55 {
		t.Fatal(audit, err)
	}
}
