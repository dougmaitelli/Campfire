package cli

import (
	"testing"

	"campfire/internal/shared/cliutil"
	"campfire/internal/shared/settings"
)

func TestNotificationConfigurationValidation(t *testing.T) {
	for _, item := range []struct {
		setting cliutil.Setting
		value   string
	}{{settings.CampfireAlertCPUPercent, "NaN"}, {settings.CampfireAlertMemoryPercent, "101"}, {settings.CampfireAlertDiskPercent, "-1"}, {settings.CampfireAlertDurationSeconds, "999999999999"}, {settings.CampfireAlertDurationSeconds, "-1"}, {settings.CampfireAlertHealthEnabled, "bad"}, {settings.CampfireAppriseURL, "file:///private-token"}} {
		t.Run(item.setting.EnvironmentName, func(t *testing.T) {
			t.Setenv(item.setting.EnvironmentName, item.value)

			if _, _, err := notificationConfig(); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}
