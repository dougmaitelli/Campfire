package cli

import (
	"flag"
	"fmt"
	"strconv"
	"strings"
	"time"

	"campfire/internal/campfire/service"
	"campfire/internal/shared/cliutil"
	"campfire/internal/shared/settings"
)

func notificationConfig(flags *flag.FlagSet) (service.AlertConfig, *service.AppriseClient, error) {
	config := service.DefaultAlertConfig()
	for _, item := range []struct {
		setting cliutil.Setting
		target  *float64
	}{{settings.CampfireAlertCPUPercent, &config.CPUPercent}, {settings.CampfireAlertMemoryPercent, &config.MemoryPercent}, {settings.CampfireAlertDiskPercent, &config.DiskPercent}} {
		value, err := strconv.ParseFloat(settingValue(flags, item.setting, strconv.FormatFloat(*item.target, 'g', -1, 64)), 64)
		if err != nil {
			return config, nil, fmt.Errorf("%s must be a percentage", item.setting.EnvironmentName)
		}

		*item.target = value
	}

	duration, err := strconv.ParseInt(settingValue(flags, settings.CampfireAlertDurationSeconds, strconv.FormatInt(int64(config.Duration.Seconds()), 10)), 10, 64)
	if err != nil || duration < 0 || duration > 86400 {
		return config, nil, fmt.Errorf("%s is outside its supported range", settings.CampfireAlertDurationSeconds.EnvironmentName)
	}

	config.Duration = time.Duration(duration) * time.Second

	health, err := strconv.ParseBool(settingValue(flags, settings.CampfireAlertHealthEnabled, strconv.FormatBool(config.Health)))
	if err != nil {
		return config, nil, fmt.Errorf("%s must be a boolean", settings.CampfireAlertHealthEnabled.EnvironmentName)
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
	client, err := service.NewAppriseClient(service.AppriseConfig{URL: settingValue(flags, settings.CampfireAppriseURL, ""), URLs: split(settingValue(flags, settings.CampfireAppriseURLs, "")), Tags: split(settingValue(flags, settings.CampfireAppriseTags, ""))})

	return config, client, err
}
