package cli

import (
	"testing"
	"time"
)

func TestResourceHistoryConfiguration(t *testing.T) {
	t.Setenv("CAMPFIRE_RESOURCE_INTERVAL_SECONDS", "10")
	t.Setenv("CAMPFIRE_RESOURCE_RETENTION_DAYS", "30")

	interval, retention, err := resourceHistoryConfig()
	if err != nil || interval != 10*time.Second || retention != 30*24*time.Hour {
		t.Fatal(interval, retention, err)
	}

	for _, values := range [][2]string{{"0", "30"}, {"301", "30"}, {"bad", "30"}, {"10", "0"}, {"10", "366"}, {"10", "bad"}} {
		t.Setenv("CAMPFIRE_RESOURCE_INTERVAL_SECONDS", values[0])
		t.Setenv("CAMPFIRE_RESOURCE_RETENTION_DAYS", values[1])

		if _, _, err := resourceHistoryConfig(); err == nil {
			t.Fatal("invalid resource configuration accepted", values)
		}
	}
}
