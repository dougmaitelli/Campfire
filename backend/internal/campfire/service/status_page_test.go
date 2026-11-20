package service

import (
	"testing"
	"time"

	"campfire/internal/campfire/statuspage"
	"campfire/internal/shared/model"
)

func TestPublicAvailabilityDoesNotInventOnlineOrOffline(t *testing.T) {
	for _, tc := range []struct {
		state model.RuntimeState
		ready bool
		want  statuspage.State
	}{
		{model.RuntimeStateRunning, true, statuspage.Online},
		{model.RuntimeStateRunning, false, statuspage.Offline},
		{model.RuntimeStateStopped, false, statuspage.Offline},
		{model.RuntimeStateStarting, false, statuspage.Offline},
		{model.RuntimeStateRestarting, false, statuspage.Offline},
		{model.RuntimeStateNotInstalled, false, statuspage.Offline},
		{model.RuntimeStateUnknown, true, statuspage.Unknown},
		{model.RuntimeStateOffline, true, statuspage.Unknown},
		{model.RuntimeState("future-state"), true, statuspage.Unknown},
	} {
		if got := publicAvailability(model.RuntimeStatus{State: tc.state, Ready: tc.ready}); got != tc.want {
			t.Fatal(tc, got)
		}
	}
}

func TestCachedRuntimeSnapshotsOwnLockingAndKeepTargetOrder(t *testing.T) {
	d := &DeploymentService{}
	d.cacheStatus("node", model.RuntimeStatus{ServerID: "game", State: model.RuntimeStateRunning, Ready: true})
	d.cacheStatus("node", model.RuntimeStatus{ServerID: "stale", State: model.RuntimeStateRunning, Ready: true})
	d.mu.Lock()
	d.observed["stale"] = time.Now().Add(-runtimeStatusLifetime)
	d.mu.Unlock()

	targets := []model.ResourceTarget{{ID: "game", SparkID: "node"}, {ID: "missing", SparkID: "node"}, {ID: "game", SparkID: "other-node"}, {ID: "stale", SparkID: "node"}}

	observations := d.cachedRuntimeStatuses(targets)
	if len(observations) != len(targets) {
		t.Fatal(observations)
	}

	for index, observation := range observations {
		if observation.Status.ServerID != targets[index].ID {
			t.Fatal("target order changed", observations)
		}

		if index == 0 {
			if observation.Status.State != model.RuntimeStateRunning || observation.Observed.IsZero() || observation.ValidFor <= 0 || observation.ValidFor > runtimeStatusLifetime {
				t.Fatal(observation)
			}
		} else if observation.Status.State != model.RuntimeStateUnknown || !observation.Observed.IsZero() || observation.ValidFor != 0 {
			t.Fatal(observation)
		}
	}

	observations[0].Status.Ready = false
	if !d.cachedRuntimeStatuses(targets[:1])[0].Status.Ready {
		t.Fatal("caller mutated cached readiness")
	}
}

func TestCachedStatusFencesPlacementAndExpiresObservations(t *testing.T) {
	d := &DeploymentService{}
	d.cacheStatus("first-node", model.RuntimeStatus{ServerID: "game", State: model.RuntimeStateRunning, Ready: true})
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	for _, tc := range []struct {
		node  string
		at    time.Time
		known bool
	}{
		{"first-node", now, true}, {"other-node", now, false}, {"first-node", now.Add(runtimeStatusLifetime), false}, {"first-node", now.Add(-time.Hour), false},
	} {
		status, observed := d.cachedStatusLocked("game", tc.node, tc.at)
		if (status.State == model.RuntimeStateRunning) != tc.known || (!observed.IsZero()) != tc.known {
			t.Fatal(tc, status, observed)
		}
	}
}
