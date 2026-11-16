package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"campfire/internal/campfire/api"
	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/pagination"
	"campfire/internal/campfire/service"
	accessperm "campfire/internal/shared/permissions"
)

func TestExecutionPagesRetainDeletedServerHistoryWithoutBypassingAuthorization(t *testing.T) {
	f := newAccessFixture(t)
	spark, ember, _ := serverFixture(t, f.admin)
	serverStore := repository.NewServerRepository(f.db.ORM())
	store := repository.NewActivityRepository(f.db.ORM())
	panel := api.New(nil, nil, service.NewServerService(serverStore, nil)).WithActivity(service.NewActivityService(store, nil, f.auth))
	f.admin = authenticatedTestAPI(t, f.db, panel)
	f.panel = panel.Handler()
	owner, regular := f.user("history-owner", database.UserRole)

	for _, id := range []string{"current-server", "deleted-server"} {
		row := database.Server{ID: id, Name: id, SparkID: spark.ID, EmberID: ember.ID, OwnerUserID: &owner.ID, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
		if err := f.db.ORM().Omit("Spark", "Ember", "Owner").Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}

	for _, run := range []database.ScheduleRun{
		{ID: "current", ServerID: "current-server", ScheduleID: "current-schedule"},
		{ID: "retained-1", ServerID: "deleted-server", ScheduleID: "deleted-schedule"},
		{ID: "retained-2", ServerID: "deleted-server", ScheduleID: "deleted-schedule"},
		{ID: "manual", ServerID: "deleted-server"},
	} {
		run.CreatedAt = 1
		run.State = "succeeded"

		run.Content = "private execution command"
		if err := store.CreateRun(t.Context(), &run); err != nil {
			t.Fatal(err)
		}
	}

	check := func(t *testing.T, handler http.Handler, query string, total int64, ids ...string) {
		t.Helper()

		response := request(handler, "GET", "/api/v1/schedules/executions?"+query, "")

		var page pagination.Page[database.ScheduleRun]
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &page) != nil {
			t.Fatal(response.Code, response.Body)
		}

		if page.Items == nil || page.Pagination.TotalItems != total || page.Pagination.TotalPages != total || page.Pagination.PageSize != 1 || len(page.Items) != len(ids) {
			t.Fatal(response.Body)
		}

		for i, id := range ids {
			if page.Items[i].ID != id {
				t.Fatal(response.Body)
			}
		}

		if strings.Contains(response.Body.String(), "private execution command") {
			t.Fatal("history leaked execution content")
		}
	}
	check(t, regular, "page_size=1", 3, "retained-2")

	if err := serverStore.Delete(t.Context(), "deleted-server"); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Run(t.Context(), "retained-1"); err != nil {
		t.Fatal("server deletion removed retained execution", err)
	}

	_, all := createTestAPIToken(t, f.admin, `["*"]`)
	_, read := createTestAPIToken(t, f.admin, `["server.view","server.schedules.read"]`)
	_, limited := createTestAPIToken(t, f.admin, `["audit.view"]`)
	_, member := createTestAPIToken(t, regular, `["*"]`)
	incomplete, incompleteSecret := createTestAPIToken(t, f.admin, `["server.view","server.schedules.read"]`)
	// Authentication must not accept a read grant whose prerequisites are absent.
	if err := f.db.ORM().Model(&incomplete).Update("scopes", `["server.view","server.schedules.read"]`).Error; err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name    string
		handler http.Handler
	}{
		{"administrator", f.admin}, {"all-scope administrator token", bearerAPI(f.panel, all)}, {"schedule-read administrator token", bearerAPI(f.panel, read)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			check(t, tc.handler, "page_size=1", 3, "retained-2")
			check(t, tc.handler, "page_size=1&page=2", 3, "retained-1")
			check(t, tc.handler, "page_size=1&page=3", 3, "current")
			check(t, tc.handler, "page_size=1&page=4", 3)
			check(t, tc.handler, "page_size=1&server_id=deleted-server", 2, "retained-2")
			check(t, tc.handler, "page_size=1&server_id=deleted-server&page=2", 2, "retained-1")
			check(t, tc.handler, "page_size=1&schedule_id=deleted-schedule", 2, "retained-2")
			check(t, tc.handler, "page_size=1&server_id=current-server", 1, "current")
			check(t, tc.handler, "page_size=1&server_id=missing", 0)
		})
	}

	for _, handler := range []http.Handler{regular, bearerAPI(f.panel, member)} {
		check(t, handler, "page_size=1", 1, "current")
		check(t, handler, "page_size=1&server_id=deleted-server", 0)
		check(t, handler, "page_size=1&schedule_id=deleted-schedule", 0)
	}

	for _, secret := range []string{limited, incompleteSecret} {
		check(t, bearerAPI(f.panel, secret), "page_size=1", 0)
		check(t, bearerAPI(f.panel, secret), "page_size=1&server_id=deleted-server", 0)
	}

	_, preview := previewAs(t, f.admin, owner.ID)
	t.Run("view-as does not inherit administrator history access", func(t *testing.T) {
		check(t, preview, "page_size=1", 1, "current")
		check(t, preview, "page_size=1&server_id=deleted-server", 0)
		check(t, preview, "page_size=1&schedule_id=deleted-schedule", 0)
	})

	t.Run("retained history remains accessible with no servers", func(t *testing.T) {
		if err := serverStore.Delete(t.Context(), "current-server"); err != nil {
			t.Fatal(err)
		}

		servers, err := serverStore.ListVisible(t.Context(), 1, true)
		if err != nil || len(servers) != 0 {
			t.Fatal("expected an empty server inventory", servers, err)
		}

		for _, handler := range []http.Handler{f.admin, bearerAPI(f.panel, all), bearerAPI(f.panel, read)} {
			check(t, handler, "page_size=1", 3, "retained-2")
			check(t, handler, "page_size=1&page=2", 3, "retained-1")
			check(t, handler, "page_size=1&page=3", 3, "current")
			check(t, handler, "page_size=1&server_id=deleted-server", 2, "retained-2")
			check(t, handler, "page_size=1&server_id=current-server", 1, "current")
		}

		for _, handler := range []http.Handler{regular, preview, bearerAPI(f.panel, member), bearerAPI(f.panel, limited), bearerAPI(f.panel, incompleteSecret)} {
			check(t, handler, "page_size=1", 0)
			check(t, handler, "page_size=1&server_id=deleted-server", 0)
		}
	})
}

func TestUnrestrictedExecutionAccessRespectsEffectiveScopes(t *testing.T) {
	f := newAccessFixture(t)

	read := accessperm.Expand([]accessperm.ID{accessperm.ServerSchedulesRead})
	for _, tc := range []struct {
		name      string
		principal service.Principal
		allowed   bool
	}{
		{"administrator", service.Principal{Admin: true}, true},
		{"non-administrator", service.Principal{TokenScopes: read, Permissions: read}, false},
		{"empty scope limit", service.Principal{Admin: true, ScopeLimit: []accessperm.ID{}}, false},
		{"scope limit missing prerequisites", service.Principal{Admin: true, ScopeLimit: []accessperm.ID{accessperm.ServerView, accessperm.ServerSchedulesRead}}, false},
		{"complete scope limit", service.Principal{Admin: true, ScopeLimit: read}, true},
		{"token blocked by scope limit", service.Principal{Admin: true, APITokenID: 1, TokenScopes: read, ScopeLimit: []accessperm.ID{accessperm.ServerView}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			allowed, err := f.auth.Access().CanReadAllScheduleExecutions(t.Context(), tc.principal)
			if err != nil || allowed != tc.allowed {
				t.Fatal("unexpected unrestricted execution access", allowed, err)
			}
		})
	}
}
