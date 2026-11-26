package service

import (
	"strings"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

func TestRuntimeCrashCollectionPersistsBeforeRedactingPublicStatus(t *testing.T) {
	activity, db, runtime, owner := activityFixture(t)
	deployment := activity.deployment
	deployment.Crashes = repository.NewCrashRepository(db.ORM())
	record := model.CrashRecord{ID: strings.Repeat("a", 64), ContainerID: strings.Repeat("b", 64), ExitedAt: time.Now().Unix(), ExitCode: 1, Reason: model.CrashReasonUnknown, Console: "private crash evidence"}

	runtime.status.Crashes = []model.CrashRecord{record}
	for range 2 {
		status, err := deployment.Action(t.Context(), "game", model.ServerActionStatus)
		if err != nil || len(status.Crashes) != 0 {
			t.Fatal("public action exposed crash payload", status, err)
		}
	}

	rows, err := deployment.RuntimeStatuses(t.Context(), owner)
	if err != nil || len(rows) != 1 || len(rows[0].Crashes) != 0 {
		t.Fatal("public cache exposed crash payload", rows, err)
	}

	var retained []database.ServerCrash
	if err := db.ORM().Find(&retained).Error; err != nil || len(retained) != 1 || retained[0].Record.Console != record.Console {
		t.Fatal(retained, err)
	}
	// Cache defense remains effective even if a future producer bypasses dispatch.
	deployment.cacheStatus("node", runtime.status)

	rows, err = deployment.RuntimeStatuses(t.Context(), owner)
	if err != nil || len(rows[0].Crashes) != 0 {
		t.Fatal(rows, err)
	}

	runtime.status.Crashes[0].Console = strings.Repeat("x", model.CrashConsoleBytes+1)
	if status, err := deployment.Action(t.Context(), "game", model.ServerActionStatus); err == nil || len(status.Crashes) != 0 {
		t.Fatal("invalid observation escaped", status, err)
	}
}
