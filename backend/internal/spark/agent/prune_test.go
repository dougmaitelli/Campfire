package agent

import (
	"context"
	"slices"
	"testing"
	"time"

	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
	"campfire/internal/spark/runtime"
)

type recordingPruner struct {
	*runtime.Manager
	calls int
}

func (r *recordingPruner) Prune(context.Context) (model.PruneResult, error) {
	r.calls++
	return model.PruneResult{Containers: 2, Images: 1}, nil
}

func TestPruneRequiresNegotiatedCapabilityAndDeadline(t *testing.T) {
	pruner := &recordingPruner{}
	store := &ConfigurationStore{Runtime: pruner}
	command := protocol.Command{ID: "prune", Name: protocol.PruneSpark, Deadline: time.Now().Add(time.Minute)}

	denied := store.executeNegotiated(t.Context(), "node", command, nil)
	if denied.Error == nil || denied.Error.Code != "unsupported_capability" || pruner.calls != 0 {
		t.Fatal("unnegotiated cleanup executed", denied)
	}

	response := store.executeNegotiated(t.Context(), "node", command, []string{protocol.CapabilityPrune})
	if response.Error != nil || response.Prune == nil || response.Prune.Containers != 2 || pruner.calls != 1 {
		t.Fatal("negotiated cleanup failed", response)
	}

	command.Deadline = time.Now().Add(-time.Second)

	response = store.executeNegotiated(t.Context(), "node", command, []string{protocol.CapabilityPrune})
	if response.Error == nil || response.Error.Code != "invalid_deadline" || pruner.calls != 1 {
		t.Fatal("expired cleanup executed", response)
	}

	store.Runtime = nil
	if slices.Contains(store.capabilities(), protocol.CapabilityPrune) {
		t.Fatal("cleanup advertised without a runtime implementation")
	}
}
