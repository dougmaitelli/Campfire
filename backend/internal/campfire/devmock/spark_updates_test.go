//go:build !production

package devmock

import "testing"

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
