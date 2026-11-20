package agent

import (
	"context"
	"slices"
	"testing"

	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
)

type crashRuntimeFixture struct{ watched []string }

func (r *crashRuntimeFixture) WatchCrashes(d model.Deployment) error {
	r.watched = append(r.watched, d.ServerID)
	return nil
}
func (*crashRuntimeFixture) SupportsCrashMonitoring() bool { return true }
func (*crashRuntimeFixture) Execute(context.Context, model.Deployment, model.RuntimeRequest) (model.RuntimeStatus, error) {
	return model.RuntimeStatus{}, nil
}
func (*crashRuntimeFixture) Files(context.Context, model.Deployment, model.FileRequest) (model.FileResult, error) {
	return model.FileResult{}, nil
}

func TestCrashMonitoringRestoresSavedConfigurationsWithoutLogMonitorDependency(t *testing.T) {
	runtime := &crashRuntimeFixture{}

	store := &ConfigurationStore{Directory: t.TempDir(), Runtime: runtime}
	if _, err := store.Deploy(t.Context(), "node", deploymentFixture("node", "game")); err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(runtime.watched, []string{"game"}) || !slices.Contains(store.capabilities(), protocol.CapabilityCrashRecords) {
		t.Fatal("configuration did not attach capable crash observer", runtime.watched)
	}

	runtime.watched = nil

	restored := &ConfigurationStore{Directory: store.Directory, Runtime: runtime}
	if err := restored.RestoreRuntimeMonitors("node"); err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(runtime.watched, []string{"game"}) {
		t.Fatal("restart did not restore crash monitoring", runtime.watched)
	}
}
