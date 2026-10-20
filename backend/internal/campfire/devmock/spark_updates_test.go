//go:build !production

package devmock

import (
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
	"slices"
	"testing"
)

func TestMockSparkPrune(t *testing.T) {
	runtime, db, _, _ := fixture(t)

	_, _, capabilities := runtime.SparkFeatures("mock-europe")
	if !slices.Contains(capabilities, protocol.CapabilityPrune) {
		t.Fatal("mock cleanup is not available through negotiated features")
	}

	sparks := service.NewSparkService(repository.NewSparkRepository(db.ORM()), runtime)

	if _, err := sparks.Prune(t.Context(), "mock-offline"); err == nil {
		t.Fatal("offline node accepted cleanup")
	}

	result, err := sparks.Prune(t.Context(), "mock-europe")
	if err != nil || result.Containers != 2 || result.Images != 3 || result.Volumes != 1 {
		t.Fatal("mock cleanup failed", result, err)
	}

	result, err = sparks.Prune(t.Context(), "mock-europe")
	if err != nil || result != (model.PruneResult{}) {
		t.Fatal("repeated cleanup removed the same resources", result, err)
	}
}

func TestMockSparkUpdateIsAvailableAndSurvivesWatcherRestart(t *testing.T) {
	runtime, db, auth, _ := fixture(t)

	_, before, _ := runtime.SparkFeatures("mock-europe")
	if before != "v1.0.0" {
		t.Fatal("outdated mock Spark fixture missing", before)
	}

	if err := runtime.UpdateSpark(t.Context(), "mock-offline", "v1.1.0"); err == nil {
		t.Fatal("offline fixture updated")
	}

	if err := runtime.UpdateSpark(t.Context(), "mock-europe", "v1.1.0"); err != nil {
		t.Fatal(err)
	}

	reloaded, err := New(t.Context(), db.ORM(), auth)
	if err != nil {
		t.Fatal(err)
	}

	_, after, _ := reloaded.SparkFeatures("mock-europe")
	if after != "v1.1.0" {
		t.Fatal("watcher restart reset the client update", after)
	}

	if err := reloaded.UpdateSpark(t.Context(), "mock-europe", "v1.1.0"); err == nil {
		t.Fatal("equal version update accepted")
	}
}
