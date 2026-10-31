package cli

import (
	"testing"

	"campfire/internal/shared/settings"
)

func TestHistoryRetentionConfiguration(t *testing.T) {
	for _, values := range [][2]string{{"-1", "0"}, {"0", "-1"}, {"36501", "0"}, {"0", "36501"}, {"bad", "1"}, {"1", "1.5"}, {"9223372036854775808", "0"}} {
		if _, err := parseHistoryRetention(values[0], values[1]); err == nil {
			t.Fatal("invalid history retention accepted", values)
		}
	}

	config, err := parseHistoryRetention("90", "30")
	if err != nil || config.AuditDays != 90 || config.OperationDays != 30 {
		t.Fatal("independent retention values lost", config, err)
	}

	t.Setenv(settings.CampfireAuditRetentionDays.EnvironmentName, "invalid")

	if err := Run(t.Context(), []string{"--" + settings.CampfireListen.FlagName, "127.0.0.1:0"}); err == nil {
		t.Fatal("invalid environment retention accepted")
	}

	if err := Run(t.Context(), []string{"--" + settings.Version.FlagName}); err != nil {
		t.Fatal("version command validated storage settings", err)
	}
}
