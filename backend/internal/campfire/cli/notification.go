package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"campfire/internal/campfire/service"
	"campfire/internal/shared/cliutil"
	"campfire/internal/shared/settings"
)

func notificationConfig() (service.AlertConfig, *service.AppriseClient, error) {
	config := service.DefaultAlertConfig()
	for _, item := range []struct {
		setting cliutil.Setting
		target  *float64
	}{{settings.CampfireAlertCPUPercent, &config.CPUPercent}, {settings.CampfireAlertMemoryPercent, &config.MemoryPercent}, {settings.CampfireAlertDiskPercent, &config.DiskPercent}} {
		value, err := strconv.ParseFloat(item.setting.Value(strconv.FormatFloat(*item.target, 'g', -1, 64)), 64)
		if err != nil {
			return config, nil, fmt.Errorf("%s must be a percentage", item.setting.EnvironmentName)
		}

		*item.target = value
	}

	duration, err := strconv.ParseInt(settings.CampfireAlertDurationSeconds.Value(strconv.FormatInt(int64(config.Duration.Seconds()), 10)), 10, 64)
	if err != nil || duration < 0 || duration > 86400 {
		return config, nil, fmt.Errorf("%s is outside its supported range", settings.CampfireAlertDurationSeconds.EnvironmentName)
	}

	config.Duration = time.Duration(duration) * time.Second

	health, err := settings.CampfireAlertHealthEnabled.BoolValue(config.Health)
	if err != nil {
		return config, nil, err
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
	client, err := service.NewAppriseClient(service.AppriseConfig{URL: settings.CampfireAppriseURL.Value(""), URLs: split(settings.CampfireAppriseURLs.Value("")), Tags: split(settings.CampfireAppriseTags.Value(""))})

	return config, client, err
}
