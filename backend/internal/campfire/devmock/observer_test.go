//go:build !production

package devmock

import (
	"slices"
	"testing"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	accessperm "campfire/internal/shared/permissions"
)

func checkObserverAccess(t *testing.T, db *database.Database, auth *service.AuthService) {
	t.Helper()

	session, _, err := auth.Login(t.Context(), "observer", Password, "")
	if err != nil {
		t.Fatal("observer cannot sign in", err)
	}

	principal := service.PrincipalFromSession(session)
	if principal.Admin || len(principal.Permissions) != 0 {
		t.Fatal("observer has platform management access", principal)
	}

	for _, id := range []string{"survival", "creative"} {
		permissions, err := auth.Access().ServerPermissions(t.Context(), principal, id)
		if err != nil || id == "survival" && !slices.Equal(permissions, []string{accessperm.ServerSchedulesRead, accessperm.ServerView}) || id == "creative" && len(permissions) != 0 {
			t.Fatal("unexpected observer access", id, permissions, err)
		}
	}

	schedules, err := repository.NewActivityRepository(db.ORM()).Schedules(t.Context(), principal.UserID, principal.Admin, "")
	if err != nil || len(schedules) != 1 || schedules[0].ID != "nightly-restart" {
		t.Fatal("observer cannot see the seeded schedule", schedules, err)
	}

	session, _, err = auth.Login(t.Context(), "viewer", Password, "")
	if err != nil {
		t.Fatal(err)
	}

	permissions, err := auth.Access().ServerPermissions(t.Context(), service.PrincipalFromSession(session), "survival")
	if err != nil || len(permissions) != 0 {
		t.Fatal("unassigned viewer gained server access", permissions, err)
	}
}

func TestObserverFixtureHasOnlyReadAccess(t *testing.T) {
	_, db, auth, _ := fixture(t)
	checkObserverAccess(t, db, auth)
}

func TestExistingMockDatabaseAddsObserverWithoutResettingData(t *testing.T) {
	_, db, auth, _ := fixture(t)
	if err := db.ORM().Unscoped().Where("username = ?", "observer").Delete(&database.User{}).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Delete(&seedMarker{ID: observerSeedMarkerID}).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Model(&database.Schedule{}).Where("id = ?", "nightly-restart").Update("name", "Saved schedule changes").Error; err != nil {
		t.Fatal(err)
	}

	if _, err := New(t.Context(), db.ORM(), auth); err != nil {
		t.Fatal(err)
	}

	checkObserverAccess(t, db, auth)

	if err := db.ORM().Model(&database.User{}).Where("username = ?", "observer").Update("name", "Saved observer changes").Error; err != nil {
		t.Fatal(err)
	}

	if _, err := New(t.Context(), db.ORM(), auth); err != nil {
		t.Fatal(err)
	}

	var user database.User
	if err := db.ORM().Where("username = ?", "observer").Take(&user).Error; err != nil || user.Name != "Saved observer changes" {
		t.Fatal("mock restart reset the observer", user, err)
	}

	var schedule database.Schedule
	if err := db.ORM().Take(&schedule, "id = ?", "nightly-restart").Error; err != nil || schedule.Name != "Saved schedule changes" {
		t.Fatal("adding the observer reset schedule data", schedule, err)
	}

	var users int64
	if err := db.ORM().Model(&database.User{}).Where("username = ?", "observer").Count(&users).Error; err != nil || users != 1 {
		t.Fatal("mock restart duplicated the observer", users, err)
	}
}
