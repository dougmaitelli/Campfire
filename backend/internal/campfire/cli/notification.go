package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"campfire/internal/campfire/service"
)

func notificationConfig() (service.AlertConfig, *service.AppriseClient, error) {
	config := service.DefaultAlertConfig()
	for _, item := range []struct {
		name   string
		target *float64
	}{{"CAMPFIRE_ALERT_CPU_PERCENT", &config.CPUPercent}, {"CAMPFIRE_ALERT_MEMORY_PERCENT", &config.MemoryPercent}, {"CAMPFIRE_ALERT_DISK_PERCENT", &config.DiskPercent}} {
		value, err := strconv.ParseFloat(environment(item.name, "90"), 64)
		if err != nil {
			return config, nil, fmt.Errorf("%s must be a percentage", item.name)
		}

		*item.target = value
	}

	for _, item := range []struct {
		name, defaultValue string
		target             *time.Duration
		max                int64
	}{{"CAMPFIRE_ALERT_DURATION_SECONDS", "120", &config.Duration, 86400}, {"CAMPFIRE_ALERT_INTERVAL_SECONDS", "30", &config.Interval, 3600}} {
		value, err := strconv.ParseInt(environment(item.name, item.defaultValue), 10, 64)
		if err != nil || value < 0 || value > item.max {
			return config, nil, fmt.Errorf("%s is outside its supported range", item.name)
		}

		*item.target = time.Duration(value) * time.Second
	}

	health, err := strconv.ParseBool(environment("CAMPFIRE_ALERT_HEALTH_ENABLED", "true"))
	if err != nil {
		return config, nil, fmt.Errorf("CAMPFIRE_ALERT_HEALTH_ENABLED must be a boolean")
	}

	config.Health = health
	if err := config.Validate(); err != nil {
		return config, nil, err
	}

	split := func(raw string) []string {
		items := make([]string, 0)

		for _, value := range strings.Split(raw, ",") {
			if value = strings.TrimSpace(value); value != "" {
				items = append(items, value)
			}
		}

		return items
	}
	client, err := service.NewAppriseClient(service.AppriseConfig{URL: environment("CAMPFIRE_APPRISE_URL", ""), URLs: split(environment("CAMPFIRE_APPRISE_URLS", "")), Tags: split(environment("CAMPFIRE_APPRISE_TAGS", ""))})

	return config, client, err
}
