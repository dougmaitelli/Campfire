package cli

import (
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/cliutil"
	"campfire/internal/shared/settings"
)

var runtimeSettingCases = []struct {
	setting  cliutil.Setting
	fallback string
	env      string
	flag     string
}{
	{settings.CampfireResourceIntervalSeconds, "10", "20", "30"},
	{settings.CampfireResourceRetentionDays, "30", "60", "90"},
	{settings.CampfireAlertCPUPercent, "90", "80", "70"},
	{settings.CampfireAlertMemoryPercent, "90", "81", "71"},
	{settings.CampfireAlertDiskPercent, "90", "82", "72"},
	{settings.CampfireAlertDurationSeconds, "120", "60", "45"},
	{settings.CampfireAlertHealthEnabled, "true", "true", "false"},
	{settings.CampfireAppriseURL, "", "https://example.com/notify/env", "https://example.com/notify/flag"},
	{settings.CampfireAppriseURLs, "", "json://env", "json://flag"},
	{settings.CampfireAppriseTags, "", "env", "flag"},
	{settings.CampfireS3Bucket, "", "env-bucket", "flag-bucket"},
	{settings.CampfireS3Region, "us-east-1", "us-west-1", "us-west-2"},
	{settings.CampfireS3Endpoint, "", "https://env.example.com", "https://flag.example.com"},
	{settings.CampfireS3Prefix, "", "env-prefix", "flag-prefix"},
	{settings.CampfireS3PathStyle, "false", "false", "true"},
}

func runtimeTestFlags(t *testing.T, args ...string) *flag.FlagSet {
	t.Helper()

	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	registerRuntimeSettings(flags)

	if err := flags.Parse(args); err != nil {
		t.Fatal(err)
	}

	return flags
}

func TestRuntimeSettingDefaultsAndPrecedence(t *testing.T) {
	for _, item := range runtimeSettingCases {
		t.Run(item.setting.FlagName, func(t *testing.T) {
			t.Setenv(item.setting.EnvironmentName, item.env)

			flags := runtimeTestFlags(t)
			if got := settingValue(flags, item.setting, "unused"); got != item.env {
				t.Fatalf("environment value = %q, want %q", got, item.env)
			}

			flags = runtimeTestFlags(t, "--"+item.setting.FlagName+"="+item.flag)
			if got := settingValue(flags, item.setting, "unused"); got != item.flag {
				t.Fatalf("flag value = %q, want %q", got, item.flag)
			}

			if err := os.Unsetenv(item.setting.EnvironmentName); err != nil {
				t.Fatal(err)
			}

			flags = runtimeTestFlags(t)
			if got := settingValue(flags, item.setting, "unused"); got != item.fallback {
				t.Fatalf("default value = %q, want %q", got, item.fallback)
			}
		})
	}
}

func TestRuntimeFlagsReachServiceConfiguration(t *testing.T) {
	args := make([]string, 0, len(runtimeSettingCases))
	for _, item := range runtimeSettingCases {
		t.Setenv(item.setting.EnvironmentName, item.env)

		args = append(args, "--"+item.setting.FlagName+"="+item.flag)
	}

	flags := runtimeTestFlags(t, args...)

	interval, retention, err := resourceHistoryConfig(flags)
	if err != nil || interval != 30*time.Second || retention != 90*24*time.Hour {
		t.Fatal("resource flags not applied", interval, retention, err)
	}

	alerts, apprise, err := notificationConfig(flags)
	if err != nil || apprise == nil || alerts.CPUPercent != 70 || alerts.MemoryPercent != 71 || alerts.DiskPercent != 72 || alerts.Duration != 45*time.Second || alerts.Health {
		t.Fatal("notification flags not applied", alerts, err)
	}

	storage, err := backupStorageConfig(flags)
	if err != nil || storage.Bucket != "flag-bucket" || storage.Region != "us-west-2" || storage.Endpoint != "https://flag.example.com" || storage.Prefix != "flag-prefix" || !storage.PathStyle {
		t.Fatal("storage flags not applied", storage, err)
	}
}

func TestAppriseFlagsReachDelivery(t *testing.T) {
	type notificationPayload struct {
		URLs []string `json:"urls"`
		Tags []string `json:"tag"`
	}

	messages := make(chan notificationPayload, 1)

	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload notificationPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}

		messages <- payload

		w.WriteHeader(http.StatusNoContent)
	}))
	defer remote.Close()

	flags := runtimeTestFlags(t, "--apprise-url="+remote.URL, "--apprise-urls=json://one, json://two", "--apprise-tags= admins, games ")

	_, client, err := notificationConfig(flags)
	if err != nil || client == nil {
		t.Fatal(client, err)
	}

	if err := client.Send(t.Context(), database.Notification{Title: "Test"}); err != nil {
		t.Fatal(err)
	}

	payload := <-messages
	if !reflect.DeepEqual(payload.URLs, []string{"json://one", "json://two"}) || !reflect.DeepEqual(payload.Tags, []string{"admins", "games"}) {
		t.Fatal("Apprise routing flags not delivered", payload)
	}
}

func TestRuntimeFlagValidation(t *testing.T) {
	for _, args := range [][]string{
		{"--resource-interval-seconds=0"}, {"--resource-retention-days=366"},
		{"--alert-cpu-percent=NaN"}, {"--alert-memory-percent=101"},
		{"--alert-disk-percent=-1"}, {"--alert-duration-seconds=-1"},
		{"--apprise-url=file:///private"}, {"--alert-health-enabled=bad"},
		{"--s3-path-style=bad"},
	} {
		t.Run(args[0], func(t *testing.T) {
			flags := flag.NewFlagSet("test", flag.ContinueOnError)
			registerRuntimeSettings(flags)

			if err := flags.Parse(args); err != nil {
				return
			}

			_, _, resourcesErr := resourceHistoryConfig(flags)
			_, _, alertsErr := notificationConfig(flags)

			_, storageErr := backupStorageConfig(flags)
			if resourcesErr == nil && alertsErr == nil && storageErr == nil {
				t.Fatal("invalid CLI value accepted")
			}
		})
	}
}

func TestRuntimeFlagsOverrideInvalidEnvironmentAndAllowEmptyValues(t *testing.T) {
	t.Setenv(settings.CampfireResourceIntervalSeconds.EnvironmentName, "bad")
	t.Setenv(settings.CampfireResourceRetentionDays.EnvironmentName, "30")
	t.Setenv(settings.CampfireAlertCPUPercent.EnvironmentName, "bad")
	t.Setenv(settings.CampfireAlertHealthEnabled.EnvironmentName, "bad")
	t.Setenv(settings.CampfireS3PathStyle.EnvironmentName, "bad")
	t.Setenv(settings.CampfireS3Bucket.EnvironmentName, "env-bucket")
	t.Setenv(settings.CampfireAppriseURL.EnvironmentName, "file:///invalid")

	flags := runtimeTestFlags(t, "--resource-interval-seconds=10", "--alert-cpu-percent=0", "--alert-health-enabled", "--s3-path-style=false", "--s3-bucket=", "--apprise-url=")
	if _, _, err := resourceHistoryConfig(flags); err != nil {
		t.Fatal(err)
	}

	alerts, client, err := notificationConfig(flags)
	if err != nil || !alerts.Health || alerts.CPUPercent != 0 || client != nil {
		t.Fatal(alerts, client, err)
	}

	storage, err := backupStorageConfig(flags)
	if err != nil || storage.PathStyle || storage.Bucket != "" {
		t.Fatal(storage, err)
	}
}

func TestRuntimeFlagsRegisteredByPanel(t *testing.T) {
	args := make([]string, 0, 1+len(runtimeSettingCases))
	args = append(args, "--version")

	for _, item := range runtimeSettingCases {
		args = append(args, "--"+item.setting.FlagName+"="+item.flag)
	}

	if err := Run(t.Context(), args); err != nil {
		t.Fatal(err)
	}
}
