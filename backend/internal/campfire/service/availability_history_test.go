package service

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/statuspage"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

func TestPublicPageReadsOnlyPublishedHistoryWithoutSampling(t *testing.T) {
	activity, db, _, _ := activityFixture(t)
	runtime := &historyRuntime{}
	activity.deployment.Remote = runtime
	store := repository.NewStatusPageRepository(db.ORM())
	_, err := store.Save(t.Context(), "", statuspage.Input{Title: "Community", Slug: "community", Entries: []statuspage.Selection{{ServerID: "game", Name: "Public game"}}})
	if err != nil {
		t.Fatal(err)
	}
	end := time.Now().Unix() / 300 * 300
	row := database.ServerResourceBucket{ServerID: "game", ResourceAggregate: database.ResourceAggregate{BucketStart: end - 300, OnlineSeconds: 240, OfflineSeconds: 30}}
	if err := db.ORM().Omit("Server").Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	service := StatusPageService{Store: store, Deployment: activity.deployment}
	page, err := service.Public(t.Context(), "community")
	if err != nil || len(page.Entries) != 1 {
		t.Fatal(page, err)
	}
	entry := page.Entries[0]
	if entry.State != statuspage.Unknown || entry.Name != "Public game" || len(entry.History.Points) != 288 {
		t.Fatal(entry)
	}
	index := (end - 300 - entry.History.From) / 300
	if got := entry.History.Points[index]; got.OnlineSeconds != 240 || got.OfflineSeconds != 30 {
		t.Fatal(got)
	}
	if runtime.calls.Load() != 0 {
		t.Fatal("public page sampled Spark")
	}
}

func availabilitySnapshot(at int64, state model.RuntimeState, ready bool) []runtimeStatusObservation {
	return []runtimeStatusObservation{{Status: model.RuntimeStatus{ServerID: "game", State: state, Ready: ready}, Observed: time.Unix(at, 0), ValidFor: runtimeStatusLifetime}}
}

func TestLifecycleAvailabilityPreservesCoverageAcrossBucketsAtEveryResourceInterval(t *testing.T) {
	for _, seconds := range []int{1, 10, 60, 300} {
		t.Run(fmt.Sprint(seconds), func(t *testing.T) {
			activity, _, _, _ := activityFixture(t)
			activity.deployment.History.Interval = time.Duration(seconds) * time.Second
			collector := NewRuntimeStatusService(activity.deployment)
			targets := []model.ResourceTarget{{ID: "game", SparkID: "node"}}
			const base int64 = 1800000000
			// Deliberately offset the lifecycle ticks from the five-minute boundaries.
			// No resource collection runs: even a five-minute metric interval must not
			// restrict the lifecycle history's coverage to its 15-second cache lifetime.
			for at := base - 5; at <= base+601; at += 3 {
				if err := collector.recordAvailability(t.Context(), targets, availabilitySnapshot(at, model.RuntimeStateRunning, true), time.Unix(at, 0)); err != nil {
					t.Fatal(err)
				}
			}
			result, err := activity.deployment.History.Store.Availability(t.Context(), []string{"game"}, base+600)
			if err != nil {
				t.Fatal(err)
			}
			for _, point := range result["game"].Points[286:] {
				if point.OnlineSeconds != 300 || point.OfflineSeconds != 0 {
					t.Fatal("continuous observations lost coverage", point)
				}
			}
		})
	}
}

func TestLifecycleAvailabilityExpiresGapsAndClassifiesReadiness(t *testing.T) {
	activity, _, _, _ := activityFixture(t)
	collector := NewRuntimeStatusService(activity.deployment)
	targets := []model.ResourceTarget{{ID: "game", SparkID: "node"}}
	const base int64 = 1800000000
	record := func(at int64, state model.RuntimeState, ready bool) {
		t.Helper()
		if err := collector.recordAvailability(t.Context(), targets, availabilitySnapshot(at, state, ready), time.Unix(at, 0)); err != nil {
			t.Fatal(err)
		}
	}
	record(base, model.RuntimeStateRunning, true)
	record(base+3, model.RuntimeStateRunning, false)  // Three seconds online.
	record(base+6, model.RuntimeStateStopped, false)  // Three seconds not ready.
	record(base+9, model.RuntimeStateUnknown, false)  // Three seconds stopped.
	record(base+12, model.RuntimeStateRunning, true)  // Three seconds unknown.
	record(base+60, model.RuntimeStateRunning, true)  // Only 15 seconds of the gap were fresh.
	record(base+63, model.RuntimeStateOffline, false) // Agent-offline is unknown, not stopped.
	record(base+66, model.RuntimeStateRunning, true)
	record(base+66, model.RuntimeStateRunning, true) // Repeated collection adds no seconds.
	// A new process must not fill the downtime since its previous observation.
	collector = NewRuntimeStatusService(activity.deployment)
	record(base+100, model.RuntimeStateRunning, true)
	record(base+103, model.RuntimeStateUnknown, false)
	result, err := activity.deployment.History.Store.Availability(t.Context(), []string{"game"}, base+300)
	if err != nil {
		t.Fatal(err)
	}
	if point := result["game"].Points[287]; point.OnlineSeconds != 24 || point.OfflineSeconds != 6 {
		t.Fatal(point)
	}
}

func TestLifecycleAvailabilityDoesNotReuseStaleOrChangedPlacement(t *testing.T) {
	activity, _, _, _ := activityFixture(t)
	collector := NewRuntimeStatusService(activity.deployment)
	targets := []model.ResourceTarget{{ID: "game", SparkID: "node"}}
	const base int64 = 1800000000
	stale := availabilitySnapshot(base-60, model.RuntimeStateRunning, true)
	if err := collector.recordAvailability(t.Context(), targets, stale, time.Unix(base, 0)); err != nil {
		t.Fatal(err)
	}
	if err := collector.recordAvailability(t.Context(), targets, availabilitySnapshot(base+3, model.RuntimeStateRunning, true), time.Unix(base+3, 0)); err != nil {
		t.Fatal(err)
	}
	targets[0].SparkID = "other-node"
	if err := collector.recordAvailability(t.Context(), targets, availabilitySnapshot(base+6, model.RuntimeStateUnknown, false), time.Unix(base+6, 0)); err != nil {
		t.Fatal(err)
	}
	result, err := activity.deployment.History.Store.Availability(t.Context(), []string{"game"}, base+300)
	if err != nil {
		t.Fatal(err)
	}
	if point := result["game"].Points[287]; point != (model.AvailabilityPoint{}) {
		t.Fatal(point)
	}
}

func TestLifecycleAvailabilityRetriesFailedWritesAndFencesClockRollback(t *testing.T) {
	activity, db, _, _ := activityFixture(t)
	collector := NewRuntimeStatusService(activity.deployment)
	targets := []model.ResourceTarget{{ID: "game", SparkID: "node"}}
	const base int64 = 1800000000
	fail := false
	const callback = "test:availability-write-failure"
	if err := db.ORM().Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if fail && tx.Statement.Table == "server_resource_buckets" {
			_ = tx.AddError(errors.New("simulated write failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.ORM().Callback().Create().Remove(callback) })
	record := func(at int64) error {
		return collector.recordAvailability(t.Context(), targets, availabilitySnapshot(at, model.RuntimeStateRunning, true), time.Unix(at, 0))
	}
	if err := record(base); err != nil {
		t.Fatal(err)
	}
	fail = true
	if err := record(base + 3); err == nil {
		t.Fatal("write failure was ignored")
	}
	if collector.availability["game"].at != base {
		t.Fatal("failed write advanced cursor")
	}
	fail = false
	for _, at := range []int64{base + 6, base + 4, base + 9} {
		if err := record(at); err != nil {
			t.Fatal(err)
		}
	}
	result, err := activity.deployment.History.Store.Availability(t.Context(), []string{"game"}, base+300)
	if err != nil {
		t.Fatal(err)
	}
	if point := result["game"].Points[287]; point.OnlineSeconds != 9 || point.OfflineSeconds != 0 {
		t.Fatal("retry or clock rollback duplicated coverage", point)
	}
}

func TestRuntimeCollectorPersistsAvailabilityWithoutResourceCollection(t *testing.T) {
	activity, db, runtime, _ := activityFixture(t)
	deployment := activity.deployment
	connected := &activityConnectivity{}
	connected.online.Store(true)
	deployment.Sparks.connectivity = connected
	deployment.History.Interval = 300 * time.Second
	runtime.status = model.RuntimeStatus{ServerID: "game", State: model.RuntimeStateRunning, Ready: true}
	collector := NewRuntimeStatusService(deployment)
	if err := collector.Collect(t.Context()); err != nil {
		t.Fatal(err)
	}
	previous := collector.availability["game"]
	if previous.state != statuspage.Online {
		t.Fatal(previous)
	}
	// Move the cursor back to exercise elapsed coverage without sleeping in tests.
	previous.at -= 3
	collector.availability["game"] = previous
	if err := collector.Collect(t.Context()); err != nil {
		t.Fatal(err)
	}
	var online int64
	if err := db.ORM().Model(&database.ServerResourceBucket{}).Select("COALESCE(SUM(online_seconds),0)").Scan(&online).Error; err != nil || online < 3 {
		t.Fatal(online, err)
	}
	var samples int64
	if err := db.ORM().Model(&database.ServerResourceSample{}).Count(&samples).Error; err != nil || samples != 0 {
		t.Fatal("runtime history required resource sampling", samples, err)
	}
}
