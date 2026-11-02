package service

import (
	"testing"
	"time"

	"campfire/internal/shared/model"
	accessperm "campfire/internal/shared/permissions"
)

func TestRuntimeStatusCacheRejectsStalePlacementAndConsoleContent(t *testing.T) {
	activity, _, _, owner := activityFixture(t)
	deployment := activity.deployment
	status := model.RuntimeStatus{ServerID: "game", State: model.RuntimeStateRunning, Ready: true, Logs: "private console", Metrics: &model.ResourceUsage{ServerID: "game"}}
	deployment.cacheStatus("node", status)

	rows, err := deployment.RuntimeStatuses(t.Context(), owner)
	if err != nil || len(rows) != 1 || rows[0].State != model.RuntimeStateRunning || rows[0].Logs != "" || rows[0].Metrics != nil {
		t.Fatal("cached status exposed console content or lost state", rows, err)
	}

	deployment.observed["game"] = time.Now().Add(-runtimeStatusLifetime)

	rows, err = deployment.RuntimeStatuses(t.Context(), owner)
	if err != nil || rows[0].State != model.RuntimeStateUnknown {
		t.Fatal("expired status remained current", rows, err)
	}

	deployment.cacheStatus("previous-node", status)

	rows, err = deployment.RuntimeStatuses(t.Context(), owner)
	if err != nil || rows[0].State != model.RuntimeStateUnknown {
		t.Fatal("status crossed Spark placement", rows, err)
	}

	owner.APITokenID = 1
	owner.TokenScopes = []model.PermissionID{accessperm.ServersCreate}

	rows, err = deployment.RuntimeStatuses(t.Context(), owner)
	if err != nil || len(rows) != 0 {
		t.Fatal("API token scopes leaked cached status", rows, err)
	}
}
