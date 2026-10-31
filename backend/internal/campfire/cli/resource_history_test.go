package cli

import (
	"testing"
	"time"

	"campfire/internal/shared/settings"
)

func TestResourceHistoryConfiguration(t *testing.T) {
	t.Setenv(settings.CampfireResourceIntervalSeconds.EnvironmentName, "10")
	t.Setenv(settings.CampfireResourceRetentionDays.EnvironmentName, "30")

	interval, retention, err := resourceHistoryConfig()
	if err != nil || interval != 10*time.Second || retention != 30*24*time.Hour {
		t.Fatal(interval, retention, err)
	}

	for _, values := range [][2]string{{"0", "30"}, {"301", "30"}, {"bad", "30"}, {"10", "0"}, {"10", "366"}, {"10", "bad"}} {
		t.Setenv(settings.CampfireResourceIntervalSeconds.EnvironmentName, values[0])
		t.Setenv(settings.CampfireResourceRetentionDays.EnvironmentName, values[1])

		if _, _, err := resourceHistoryConfig(); err == nil {
			t.Fatal("invalid resource configuration accepted", values)
		}
	}
}
