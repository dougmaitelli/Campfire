package cli

import (
	"fmt"
	"strconv"
	"time"

	"campfire/internal/campfire/service"
)

func resourceHistoryConfig() (time.Duration, time.Duration, error) {
	interval, err := strconv.ParseInt(environment("CAMPFIRE_RESOURCE_INTERVAL_SECONDS", fmt.Sprint(int64(service.DefaultResourceInterval.Seconds()))), 10, 64)
	if err != nil || interval < 1 || interval > 300 {
		return 0, 0, fmt.Errorf("CAMPFIRE_RESOURCE_INTERVAL_SECONDS must be between 1 and 300")
	}

	retention, err := strconv.ParseInt(environment("CAMPFIRE_RESOURCE_RETENTION_DAYS", fmt.Sprint(int64(service.DefaultResourceRetention/(24*time.Hour)))), 10, 64)
	if err != nil || retention < 1 || retention > 365 {
		return 0, 0, fmt.Errorf("CAMPFIRE_RESOURCE_RETENTION_DAYS must be between 1 and 365")
	}

	return time.Duration(interval) * time.Second, time.Duration(retention) * 24 * time.Hour, nil
}
