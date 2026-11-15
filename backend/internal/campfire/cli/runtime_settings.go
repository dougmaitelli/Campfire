package cli

import (
	"flag"
	"fmt"
	"strconv"
	"time"

	"campfire/internal/campfire/backupstorage"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/cliutil"
	"campfire/internal/shared/settings"
)

// registerRuntimeSettings adds environment-backed monitoring and delivery flags.
func registerRuntimeSettings(flags *flag.FlagSet) {
	defaults := service.DefaultAlertConfig()
	for _, item := range []struct {
		setting cliutil.Setting
		value   string
		usage   string
	}{
		{settings.CampfireResourceIntervalSeconds, strconv.FormatInt(int64(service.DefaultResourceInterval/time.Second), 10), "resource collection interval in seconds, 1–300"},
		{settings.CampfireResourceRetentionDays, strconv.FormatInt(int64(service.DefaultResourceRetention/(24*time.Hour)), 10), "resource history retention in days, 1–365"},
		{settings.CampfireAlertCPUPercent, strconv.FormatFloat(defaults.CPUPercent, 'g', -1, 64), "CPU alert percentage; 0 disables"},
		{settings.CampfireAlertMemoryPercent, strconv.FormatFloat(defaults.MemoryPercent, 'g', -1, 64), "memory alert percentage; 0 disables"},
		{settings.CampfireAlertDiskPercent, strconv.FormatFloat(defaults.DiskPercent, 'g', -1, 64), "disk alert percentage; 0 disables"},
		{settings.CampfireAlertDurationSeconds, strconv.FormatInt(int64(defaults.Duration/time.Second), 10), "alert and recovery persistence in seconds, 0–86400"},
		{settings.CampfireAppriseURL, "", "complete Apprise notify endpoint"},
		{settings.CampfireAppriseURLs, "", "comma-separated Apprise target URLs"},
		{settings.CampfireAppriseTags, "", "comma-separated Apprise routing tags"},
		{settings.CampfireS3Bucket, "", "S3 backup bucket"},
		{settings.CampfireS3Region, "us-east-1", "S3 region"},
		{settings.CampfireS3Endpoint, "", "S3-compatible service endpoint"},
		{settings.CampfireS3Prefix, "", "S3 backup object-key prefix"},
	} {
		item.setting.String(flags, item.value, item.usage)
	}

	for _, item := range []struct {
		setting cliutil.Setting
		value   bool
		usage   string
	}{
		{settings.CampfireAlertHealthEnabled, defaults.Health, "enable Spark and server health alerts"},
		{settings.CampfireS3PathStyle, false, "use path-style S3 addressing"},
	} {
		value := booleanSetting(item.setting.Value(strconv.FormatBool(item.value)))
		flags.Var(&value, item.setting.FlagName, item.usage+" ("+item.setting.EnvironmentName+")")
	}
}

// Validate environment booleans after parsing so an explicit flag can override
// an invalid environment value, just as it can for numeric and string settings.
type booleanSetting string

func (value *booleanSetting) String() string { return string(*value) }

func (value *booleanSetting) IsBoolFlag() bool { return true }

func (value *booleanSetting) Set(raw string) error {
	if _, err := strconv.ParseBool(raw); err != nil {
		return err
	}

	*value = booleanSetting(raw)

	return nil
}

// settingValue reads the parsed flag, falling back to the environment for callers
// that do not register command-line flags.
func settingValue(flags *flag.FlagSet, setting cliutil.Setting, fallback string) string {
	if flags != nil {
		if value := flags.Lookup(setting.FlagName); value != nil {
			return value.Value.String()
		}
	}

	return setting.Value(fallback)
}

func backupStorageConfig(flags *flag.FlagSet) (backupstorage.Config, error) {
	pathStyle, err := strconv.ParseBool(settingValue(flags, settings.CampfireS3PathStyle, "false"))
	if err != nil {
		return backupstorage.Config{}, fmt.Errorf("%s must be a boolean", settings.CampfireS3PathStyle.EnvironmentName)
	}

	return backupstorage.Config{
		Bucket:    settingValue(flags, settings.CampfireS3Bucket, ""),
		Region:    settingValue(flags, settings.CampfireS3Region, "us-east-1"),
		Endpoint:  settingValue(flags, settings.CampfireS3Endpoint, ""),
		Prefix:    settingValue(flags, settings.CampfireS3Prefix, ""),
		PathStyle: pathStyle,
	}, nil
}
