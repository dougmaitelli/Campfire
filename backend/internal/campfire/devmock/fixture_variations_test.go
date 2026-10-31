//go:build !production

package devmock

import (
	"slices"
	"testing"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
	accessperm "campfire/internal/shared/permissions"
)

func TestMockFixturesCoverDistinctServerStatesAndAccess(t *testing.T) {
	_, db, auth, servers := fixture(t)

	for _, server := range servers {
		if server.EmberUpdateAvailable != (server.Name == mockUpdateServerName) {
			t.Fatal("only the update example should have an unapplied Ember", server.Name, server.EmberUpdateAvailable)
		}
	}

	for _, item := range []struct {
		username  string
		serverID  string
		sparkID   string
		provision bool
		allowed   string
		denied    string
	}{
		{"alex", "survival", "mock-europe", true, accessperm.ServerDelete, ""},
		{"sam", "creative", "mock-america", true, accessperm.ServerBackupsCreate, accessperm.ServerConfigure},
		{"jordan", "modded", "mock-america", false, accessperm.ServerOperate, accessperm.ServerConfigure},
		{"morgan", "modded", "mock-europe", true, accessperm.ServerConfigure, accessperm.ServerDelete},
		{"observer", "survival", "mock-europe", false, accessperm.ServerSchedulesRead, accessperm.ServerFilesRead},
		{"viewer", "survival", "mock-europe", false, "", accessperm.ServerView},
	} {
		_, token, err := auth.Login(t.Context(), item.username, Password, "")
		if err != nil {
			t.Fatal(item.username, err)
		}

		session, _, err := auth.Session(t.Context(), token)
		if err != nil {
			t.Fatal(item.username, err)
		}

		permissions, err := auth.Access().ServerPermissions(t.Context(), service.PrincipalFromSession(session), item.serverID)
		if err != nil || item.allowed != "" && !slices.Contains(permissions, item.allowed) || item.denied != "" && slices.Contains(permissions, item.denied) {
			t.Fatal("mock account does not exercise its intended access", item.username, permissions, err)
		}

		allowed, err := auth.Access().CanProvision(t.Context(), service.PrincipalFromSession(session), item.sparkID)
		if err != nil || allowed != item.provision {
			t.Fatal("mock account does not exercise its intended placement access", item.username, allowed, err)
		}
	}

	activity := repository.NewActivityRepository(db.ORM())

	schedules, err := activity.Schedules(t.Context(), 1, true, "")
	if err != nil || len(schedules) != 2 {
		t.Fatal(schedules, err)
	}

	for _, schedule := range schedules {
		if schedule.Action == "backup" && (schedule.BackupMode != model.BackupModeSavedata || schedule.Enabled || schedule.CronExpression != "") {
			t.Fatal("backup example must be a paused one-off save-data schedule", schedule)
		}
	}

	runs, err := activity.ScheduleRuns(t.Context(), 1, true, "survival", "", "nightly-restart")
	if err != nil || len(runs) != 2 {
		t.Fatal("expected successful and failed executions for the same schedule", runs, err)
	}

	states := make(map[string]bool)
	for _, run := range runs {
		states[run.State] = true
	}

	if !states["failed"] || !states["succeeded"] {
		t.Fatal("missing execution outcome variation", states)
	}
}
