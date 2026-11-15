package cli

import (
	"flag"
	"fmt"
	"strconv"
	"time"

	"campfire/internal/campfire/service"
	"campfire/internal/shared/settings"
)

func resourceHistoryConfig(flags *flag.FlagSet) (time.Duration, time.Duration, error) {
	interval, err := strconv.ParseInt(settingValue(flags, settings.CampfireResourceIntervalSeconds, fmt.Sprint(int64(service.DefaultResourceInterval.Seconds()))), 10, 64)
	if err != nil || interval < 1 || interval > 300 {
		return 0, 0, fmt.Errorf("%s must be between 1 and 300", settings.CampfireResourceIntervalSeconds.EnvironmentName)
	}

	retention, err := strconv.ParseInt(settingValue(flags, settings.CampfireResourceRetentionDays, fmt.Sprint(int64(service.DefaultResourceRetention/(24*time.Hour)))), 10, 64)
	if err != nil || retention < 1 || retention > 365 {
		return 0, 0, fmt.Errorf("%s must be between 1 and 365", settings.CampfireResourceRetentionDays.EnvironmentName)
	}

	return time.Duration(interval) * time.Second, time.Duration(retention) * 24 * time.Hour, nil
}
