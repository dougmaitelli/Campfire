package runtime

import (
	"testing"

	"campfire/internal/shared/model"
)

func TestSelfUpdateRejectsActiveWorkAndTransferFences(t *testing.T) {
	manager := New(t.Context(), t.TempDir())
	t.Cleanup(manager.Close)

	if err := manager.CanUpdate(); err != nil {
		t.Fatal(err)
	}

	manager.entries["server"] = &entry{Status: model.RuntimeStatus{Busy: true}}
	if err := manager.CanUpdate(); err == nil {
		t.Fatal("busy operation allowed an update")
	}

	manager.entries["server"].Status.Busy = false
	if err := manager.saveTransfer(&localTransfer{TransferResult: model.TransferResult{ID: "transfer", ServerID: "server", State: "ready"}, Role: "source"}); err != nil {
		t.Fatal(err)
	}

	if err := manager.CanUpdate(); err == nil {
		t.Fatal("active transfer fence allowed an update")
	}
}
