package cli

import "testing"

func TestNotificationConfigurationValidation(t *testing.T) {
	for _, item := range []struct{ name, value string }{{"CAMPFIRE_ALERT_CPU_PERCENT", "NaN"}, {"CAMPFIRE_ALERT_MEMORY_PERCENT", "101"}, {"CAMPFIRE_ALERT_DISK_PERCENT", "-1"}, {"CAMPFIRE_ALERT_DURATION_SECONDS", "999999999999"}, {"CAMPFIRE_ALERT_DURATION_SECONDS", "-1"}, {"CAMPFIRE_ALERT_HEALTH_ENABLED", "bad"}, {"CAMPFIRE_APPRISE_URL", "file:///private-token"}} {
		t.Run(item.name, func(t *testing.T) {
			t.Setenv(item.name, item.value)

			if _, _, err := notificationConfig(); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}
