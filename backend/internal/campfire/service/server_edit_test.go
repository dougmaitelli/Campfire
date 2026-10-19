package service

import (
	"context"
	"testing"
	"time"

	"campfire/internal/shared/model"
)

type waitingEditGuard struct {
	*DeploymentService
	entered chan struct{}
}

func (g *waitingEditGuard) BeforeEdit(ctx context.Context, id string, input model.ServerConfiguration) (func(), error) {
	close(g.entered)

	return g.DeploymentService.BeforeEdit(ctx, id, input)
}

func hiddenEditFixture(t *testing.T) (*DeploymentService, model.Server) {
	t.Helper()

	f := newTransferFixture(t)
	d := f.service.Deployment

	ember, err := d.Embers.Get(t.Context(), f.server.EmberID)
	if err != nil {
		t.Fatal(err)
	}

	ember.Variables = []model.Variable{{Name: "PRIVATE_VALUE", Default: "original", Editable: true, Hidden: true}}
	if _, err := d.Embers.Replace(t.Context(), ember.ID, ember); err != nil {
		t.Fatal(err)
	}

	server, err := d.Servers.ApplyEmber(t.Context(), f.server.ID, ember.ID, f.server.ServerConfiguration)
	if err != nil {
		t.Fatal(err)
	}

	return d, server
}

func TestWaitingEditPreservesLatestHiddenValues(t *testing.T) {
	d, server := hiddenEditFixture(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	release, err := d.lockServer(ctx, server.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if release != nil {
			release()
		}
	}()

	guard := &waitingEditGuard{DeploymentService: d, entered: make(chan struct{})}
	d.Servers.WithRuntimeGuard(guard)

	input := server.ServerConfiguration
	input.Name, input.Variables = "Renamed", nil
	done := make(chan error, 1)

	go func() {
		_, err := d.Servers.Update(ctx, server.ID, input)
		done <- err
	}()

	select {
	case <-guard.entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	// Commit the competing edit while holding the same lifecycle lock.
	server.Variables["PRIVATE_VALUE"] = "latest"
	if err := d.Servers.store.Save(ctx, server, false); err != nil {
		t.Fatal(err)
	}

	release()
	release = nil

	if err := <-done; err != nil {
		t.Fatal(err)
	}

	saved, err := d.Servers.Get(ctx, server.ID)
	if err != nil || saved.Name != "Renamed" || saved.Variables["PRIVATE_VALUE"] != "latest" {
		t.Fatal("waiting edit restored stale hidden values", saved, err)
	}
}

func TestMetadataEditWithOmittedHiddenValuesDoesNotInspectBusyRuntime(t *testing.T) {
	d, server := hiddenEditFixture(t)
	d.Remote = &activityRuntime{status: model.RuntimeStatus{ServerID: server.ID, Busy: true}}
	input := server.ServerConfiguration
	input.Description, input.Variables = "New description", nil

	saved, err := d.Servers.Update(t.Context(), server.ID, input)
	if err != nil || saved.Description != input.Description || saved.Variables["PRIVATE_VALUE"] != "original" {
		t.Fatal("metadata edit lost the busy-runtime fast path", saved, err)
	}
}
