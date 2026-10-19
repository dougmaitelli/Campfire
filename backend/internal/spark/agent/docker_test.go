package agent

import (
	"strings"
	"testing"
)

func TestUnsupportedDockerConfigurationFailsBeforeEnrollment(t *testing.T) {
	t.Setenv("DOCKER_CONTEXT", "remote")

	err := Run(t.Context(), Config{DataDir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "DOCKER_CONTEXT") {
		t.Fatal("unsupported Docker configuration was not rejected before enrollment", err)
	}
}

func TestEnrollmentOnlyDoesNotValidateDockerConfiguration(t *testing.T) {
	t.Setenv("DOCKER_CONTEXT", "remote")

	err := Run(t.Context(), Config{DataDir: t.TempDir(), EnrollOnly: true})
	if err == nil || strings.Contains(err.Error(), "DOCKER_CONTEXT") {
		t.Fatal("enrollment-only mode did not reach enrollment validation", err)
	}
}
