package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
