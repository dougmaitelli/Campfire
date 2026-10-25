package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sparkcli "campfire/internal/spark/cli"
)

func TestVersionCommandsDoNotInitializePersistentState(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "panel.db")
	t.Setenv("CAMPFIRE_DATABASE", path)
	t.Setenv("CAMPFIRE_LISTEN", "invalid-listen-address")
	t.Setenv("SPARK_DATA", filepath.Join(directory, "spark"))
	t.Setenv("SPARK_TOKEN_FILE", filepath.Join(directory, "absent-token"))

	if err := Run(context.Background(), []string{"--version"}); err != nil {
		t.Fatal("version unnecessarily validated runtime settings", err)
	}

	if err := sparkcli.Run(context.Background(), []string{"--version"}); err != nil {
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

func TestGameEventLimitsFailBeforePersistentStartup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.db")
	t.Setenv("CAMPFIRE_DATABASE", path)
	t.Setenv("CAMPFIRE_GAME_EVENT_RETENTION_DAYS", "30")
	t.Setenv("CAMPFIRE_GAME_EVENT_LIMIT", "10000")

	for _, args := range [][]string{{"--game-event-retention-days", "0"}, {"--game-event-retention-days", "3651"}, {"--game-event-limit", "0"}, {"--game-event-limit", "100001"}} {
		if err := Run(t.Context(), args); err == nil || !strings.Contains(err.Error(), "game event") {
			t.Fatal("invalid policy reached application startup", args, err)
		}
	}

	t.Setenv("CAMPFIRE_GAME_EVENT_RETENTION_DAYS", "invalid")

	if err := Run(t.Context(), []string{"--game-event-retention-days", "30", "--game-event-limit", "0"}); err == nil || !strings.Contains(err.Error(), "game event limit") {
		t.Fatal("flag did not override environment", err)
	}

	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("invalid event policy initialized persistent state", err)
	}
}
