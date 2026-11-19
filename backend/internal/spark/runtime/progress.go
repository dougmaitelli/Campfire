package runtime

import (
	"context"

	"campfire/internal/shared/model"
	"campfire/internal/shared/workprogress"
)

// Archive codecs own their counters; runtime only publishes immutable snapshots
// under the entry lock. Operation success remains owned by run after commit.
func runtimeProgress(ctx context.Context, e *entry) context.Context {
	return workprogress.WithReporter(ctx, func(snapshot workprogress.Snapshot) {
		e.mu.Lock()
		defer e.mu.Unlock()

		e.Status.WorkProgress = &snapshot
		e.Status.OperationState = model.OperationRunning
	})
}
