//go:build !production

package devmock

import (
	"os"
	"testing"

	"campfire/internal/shared/settings"
)

func TestRunRejectsArgumentsBeforeChangingEnvironment(t *testing.T) {
	key := settings.CampfireDatabase.EnvironmentName
	t.Setenv(key, "production-database-must-not-be-used")

	if err := Run(t.Context(), []string{"--database", "other.db"}); err == nil {
		t.Fatal("mock application accepted persistent-state overrides")
	}

	if os.Getenv(key) != "production-database-must-not-be-used" {
		t.Fatal("invalid invocation changed the process environment")
	}
}
