package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
)

type limitedCapabilityRouter struct {
	*transferRouter
	deliveries int
	denyDelete bool
}

func (r *limitedCapabilityRouter) Deploy(ctx context.Context, id string, deployment model.Deployment) (model.ConfigurationReceipt, error) {
	r.deliveries++
	return r.transferRouter.Deploy(ctx, id, deployment)
}

func (r *limitedCapabilityRouter) CheckRuntime(_ string, request model.RuntimeRequest) error {
	if request.Action == "reinstall" {
		return protocol.Require(nil, protocol.CapabilityReinstall)
	}

	if request.Action == "delete" && r.denyDelete {
		return protocol.Require(nil, protocol.CapabilityLifecycle)
	}

	return nil
}

func TestUnsupportedServerDeletionPreservesOffHostBackups(t *testing.T) {
	f, backups, remote, _ := backupFixture(t)
	ctx := context.Background()

	seedBackup(t, f, "protected", time.Now())

	if err := backups.SavePolicy(ctx, f.server.ID, model.BackupPolicy{Storage: "s3"}); err != nil {
		t.Fatal(err)
	}

	if err := backups.reconcile(ctx, f.server.ID); err != nil {
		t.Fatal(err)
	}

	if len(remote.objects) != 1 {
		t.Fatal("backup was not uploaded")
	}

	f.service.Deployment.Remote = &limitedCapabilityRouter{transferRouter: f.router, denyDelete: true}
	err := f.service.Deployment.Delete(ctx, f.server.ID)

	var failure *protocol.CommandError
	if !errors.As(err, &failure) || failure.Code != "unsupported_capability" {
		t.Fatal("unsupported server deletion accepted", err)
	}

	rows, err := backups.Store.List(ctx, f.server.ID)
	if err != nil || len(rows) != 1 || rows[0].Deleting || !rows[0].Remote || len(remote.objects) != 1 {
		t.Fatal("capability rejection changed backups", rows, err)
	}
}

func (*limitedCapabilityRouter) CheckTransfer(id string) error {
	if id == "target" {
		return protocol.Require(nil, protocol.CapabilityTransfer)
	}

	return nil
}

func TestCapabilityPreflightPreventsConfigurationChangesAndReservations(t *testing.T) {
	f := newTransferFixture(t)
	limited := &limitedCapabilityRouter{transferRouter: f.router}
	f.service.Deployment.Remote = limited
	_, err := f.service.Deployment.Action(context.Background(), f.server.ID, "reinstall")

	var failure *protocol.CommandError
	if !errors.As(err, &failure) || failure.Code != "unsupported_capability" || limited.deliveries != 0 {
		t.Fatal("unsupported installer action delivered configuration", limited.deliveries, err)
	}

	if _, err := f.service.Begin(context.Background(), f.server.ID, "target", f.ports); !errors.As(err, &failure) {
		t.Fatal("transfer accepted an unsupported target", err)
	}

	transfers, err := f.service.Store.List(context.Background(), f.server.ID, false)
	if err != nil || len(transfers) != 0 {
		t.Fatal("unsupported target left a transfer reservation", err)
	}
}
