package runtime

import (
	"errors"
	"fmt"
	"testing"

	"campfire/internal/shared/model"
)

func TestRuntimeErrorClassificationUsesCodes(t *testing.T) {
	failure := &runtimeFailure{code: model.RuntimeQuotaUnavailable, message: "An entirely different display message", cause: errors.New("host unavailable")}
	if code := runtimeErrorCode(fmt.Errorf("wrapped: %w", failure)); code != model.RuntimeQuotaUnavailable {
		t.Fatal("lost the typed quota error", code)
	}

	if code := runtimeErrorCode(errors.New("game-data quota unavailable: this is only text")); code != model.RuntimeOperationFailed {
		t.Fatal("classified an error by its text", code)
	}
}

func TestQuotaFailureIsReportedAsServerDiagnostic(t *testing.T) {
	m := newTestManager(t.Context(), t.TempDir())
	t.Cleanup(m.Close)
	m.Docker = &fakeDocker{states: make(map[string]string)}
	m.Quota = fakeQuota{err: quotaError(errors.New("host quota support lost"))}

	status := awaitOperation(t, m, runtimeFixture(), "install", "quota-diagnostic")
	if status.OperationState != model.OperationFailed || status.DiskQuotaErrorCode != model.RuntimeQuotaUnavailable || status.Error != "" || status.ErrorCode != "" {
		t.Fatal("quota error leaked into general operation errors", status)
	}
}
