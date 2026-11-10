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
	runtime, db, auth, servers := fixture(t)
	runtimeStates := make(map[model.RuntimeState]bool)

	var offline bool

	for _, server := range servers {
		if server.EmberReviewRequired != (server.Name == mockUpdateServerName) || server.DeploymentSyncRequired != (server.ID == syncExampleID || server.ID == firstSyncExampleID) {
			t.Fatal("fixture lost its intended review/sync distinction", server.Name, server.EmberReviewRequired, server.DeploymentSyncRequired)
		}

		if !runtime.Connected(server.SparkID) {
			offline = true
			continue
		}

		status, err := runtime.Action(t.Context(), server.SparkID, model.RuntimeRequest{ServerID: server.ID, Action: model.ServerActionStatus})
		if err != nil {
			t.Fatal(err)
		}

		runtimeStates[status.State] = true
		if server.ID == "modded" && (status.State != model.RuntimeStateNotInstalled || status.Busy) {
			t.Fatal("pending-install fixture must be ready for installation", status)
		}
	}

	if !offline || !runtimeStates[model.RuntimeStateRunning] || !runtimeStates[model.RuntimeStateStopped] || !runtimeStates[model.RuntimeStateNotInstalled] {
		t.Fatal("missing server lifecycle variation", runtimeStates, offline)
	}

	for _, item := range []struct {
		username  string
		serverID  string
		sparkID   string
		provision bool
		allowed   model.PermissionID
		denied    model.PermissionID
	}{
		{"alex", "survival", "mock-europe", true, accessperm.ServerDelete, ""},
		{"sam", "creative", "mock-america", true, accessperm.ServerBackupsCreate, accessperm.ServerConfigure},
		{"jordan", "modded", "mock-america", false, accessperm.ServerOperate, accessperm.ServerConfigure},
		{"morgan", "modded", "mock-europe", true, accessperm.ServerConfigure, accessperm.ServerDelete},
		{"observer", "survival", "mock-europe", false, accessperm.ServerSchedulesRead, accessperm.ServerFilesRead},
		{"viewer", "survival", "mock-europe", false, "", accessperm.ServerView},
		{"casey", syncExampleID, configurationSparkID, false, accessperm.ServerView, accessperm.ServerOperate},
		{"sam", syncExampleID, configurationSparkID, false, accessperm.ServerOperate, accessperm.ServerConfigure},
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

	states := make(map[model.ExecutionState]bool)
	for _, run := range runs {
		states[run.State] = true
	}

	if !states["failed"] || !states["succeeded"] {
		t.Fatal("missing execution outcome variation", states)
	}
}

func TestPendingInstallationFixtureDoesNotResetAfterInstallation(t *testing.T) {
	runtime, db, auth, _ := fixture(t)
	request := model.RuntimeRequest{ServerID: "modded", Action: model.ServerActionInstall, OperationID: "install-example"}

	status, err := runtime.Action(t.Context(), "mock-america", request)
	if err != nil || status.State != model.RuntimeStateStopped {
		t.Fatal("pending-install fixture could not be installed", status, err)
	}

	reloaded, err := New(t.Context(), db.ORM(), auth)
	if err != nil {
		t.Fatal(err)
	}

	request.Action = model.ServerActionStatus

	status, err = reloaded.Action(t.Context(), "mock-america", request)
	if err != nil || status.State != model.RuntimeStateStopped || status.OperationID != "install-example" {
		t.Fatal("restarting mock mode reset the installed fixture", status, err)
	}
}
