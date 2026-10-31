package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"campfire/internal/shared/settings"
	sparkcli "campfire/internal/spark/cli"
)

func TestVersionCommandsDoNotInitializePersistentState(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "panel.db")
	t.Setenv(settings.CampfireDatabase.EnvironmentName, path)
	t.Setenv(settings.CampfireListen.EnvironmentName, "invalid-listen-address")
	t.Setenv(settings.SparkData.EnvironmentName, filepath.Join(directory, "spark"))
	t.Setenv(settings.SparkTokenFile.EnvironmentName, filepath.Join(directory, "absent-token"))

	if err := Run(context.Background(), []string{"--" + settings.Version.FlagName}); err != nil {
		t.Fatal("version unnecessarily validated runtime settings", err)
	}

	if err := sparkcli.Run(context.Background(), []string{"--" + settings.Version.FlagName}); err != nil {
		t.Fatal("Spark version attempted enrollment", err)
	}

	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("version initialized the panel database")
	}

	if _, err := os.Stat(filepath.Join(directory, "spark")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("version initialized agent identity")
	}
}

func TestCampfireRejectsRemovedEmbeddedRuntimeFlag(t *testing.T) {
	if err := Run(context.Background(), []string{"--no-local"}); err == nil {
		t.Fatal("removed embedded runtime option was accepted")
	}
}

func TestOIDCAutoLinkRejectsInvalidEnvironment(t *testing.T) {
	t.Setenv(settings.CampfireOIDCAutoLinkEmail.EnvironmentName, "invalid")

	if err := Run(t.Context(), nil); err == nil || !strings.Contains(err.Error(), settings.CampfireOIDCAutoLinkEmail.EnvironmentName+" must be a boolean") {
		t.Fatal("invalid automatic linking policy reached startup", err)
	}
}

func TestGameEventLimitsFailBeforePersistentStartup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.db")
	t.Setenv(settings.CampfireDatabase.EnvironmentName, path)
	t.Setenv(settings.CampfireGameEventRetentionDays.EnvironmentName, "30")
	t.Setenv(settings.CampfireGameEventLimit.EnvironmentName, "10000")

	for _, args := range [][]string{{"--" + settings.CampfireGameEventRetentionDays.FlagName, "0"}, {"--" + settings.CampfireGameEventRetentionDays.FlagName, "3651"}, {"--" + settings.CampfireGameEventLimit.FlagName, "0"}, {"--" + settings.CampfireGameEventLimit.FlagName, "100001"}} {
		if err := Run(t.Context(), args); err == nil || !strings.Contains(err.Error(), "game event") {
			t.Fatal("invalid policy reached application startup", args, err)
		}
	}

	t.Setenv(settings.CampfireGameEventRetentionDays.EnvironmentName, "invalid")

	if err := Run(t.Context(), []string{"--" + settings.CampfireGameEventRetentionDays.FlagName, "30", "--" + settings.CampfireGameEventLimit.FlagName, "0"}); err == nil || !strings.Contains(err.Error(), "game event limit") {
		t.Fatal("flag did not override environment", err)
	}

	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("invalid event policy initialized persistent state", err)
	}
}
