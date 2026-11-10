package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"campfire/internal/shared/model"
)

type stagedConfigurationRouter struct {
	*transferRouter
	fail       bool
	deliveries int
}

func (r *stagedConfigurationRouter) Deploy(ctx context.Context, id string, input model.Deployment) (model.ConfigurationReceipt, error) {
	r.deliveries++
	if r.fail {
		return model.ConfigurationReceipt{}, errors.New("Spark disconnected")
	}

	return r.transferRouter.Deploy(ctx, id, input)
}

func TestRunningEmberUpdatesApplyBackupPathsWithoutRestart(t *testing.T) {
	f := newTransferFixture(t)
	d := f.service.Deployment
	ctx := t.Context()
	wait := func(pending bool) model.RuntimeStatus {
		t.Helper()

		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if err := d.syncChanged(ctx, f.server.ID); err != nil {
				t.Fatal(err)
			}

			status, err := d.Action(ctx, f.server.ID, model.ServerActionStatus)
			if err == nil && !status.Busy && status.State == model.RuntimeStateRunning && status.RestartRequired == pending {
				return status
			}

			time.Sleep(10 * time.Millisecond)
		}

		t.Fatal("server did not reach expected restart state", pending)

		return model.RuntimeStatus{}
	}

	if _, err := d.Action(ctx, f.server.ID, model.ServerActionStart); err != nil {
		t.Fatal(err)
	}

	original := wait(false)
	router := &stagedConfigurationRouter{transferRouter: f.router}
	d.Remote = router

	ember, err := d.Embers.Get(ctx, f.server.EmberID)
	if err != nil {
		t.Fatal(err)
	}

	ember.SavedataPaths = []model.SavedataPath{{Path: "world"}}
	if _, err := d.Embers.Replace(ctx, ember.ID, ember); err != nil {
		t.Fatal(err)
	}

	status := wait(false)
	if status.OperationID != original.OperationID {
		t.Fatal("applying save paths changed the running process", status)
	}

	options, err := d.backupOptions(ctx, f.server.ID, model.BackupModeSavedata)
	if err != nil || len(options.Paths) != 1 || options.Paths[0].Path != "world" {
		t.Fatal("save paths were not immediately available", options, err)
	}

	if router.deliveries != 0 {
		t.Fatal("save paths unnecessarily replaced the container deployment", router.deliveries)
	}

	input := f.server.ServerConfiguration

	input.StartupOverride = "sleep 5000"
	if _, err := d.Servers.Update(ctx, f.server.ID, input); err != nil {
		t.Fatal(err)
	}

	router.fail = true

	if err := d.syncChanged(ctx, f.server.ID); err == nil {
		t.Fatal("disconnected Spark unexpectedly acknowledged configuration")
	}

	pending, err := d.Servers.Get(ctx, f.server.ID)
	if err != nil || !pending.DeploymentSyncRequired || pending.EmberReviewRequired || pending.EmberHash != f.server.EmberHash {
		t.Fatal("failed delivery cleared the pending deployment", pending, err)
	}

	router.fail = false

	status = wait(true)
	if status.OperationID != original.OperationID {
		t.Fatal("saving container settings restarted the server", status)
	}

	for range 3 {
		wait(true)
	}

	if router.deliveries != 2 {
		t.Fatal("unchanged configuration was delivered repeatedly", router.deliveries)
	}

	if _, err := d.Action(ctx, f.server.ID, model.ServerActionStart); err != nil {
		t.Fatal(err)
	}

	wait(true)

	if _, err := d.Action(ctx, f.server.ID, model.ServerActionRestart); err != nil {
		t.Fatal(err)
	}

	wait(false)
}

func TestEmberReviewRetainsBusyAndReservationGuards(t *testing.T) {
	f := newTransferFixture(t)
	d := f.service.Deployment

	ember, err := d.Embers.Get(t.Context(), f.server.EmberID)
	if err != nil {
		t.Fatal(err)
	}

	ember.Variables = append(ember.Variables, model.Variable{Name: "NEW_SETTING", Default: "valid", Editable: true})
	if _, err := d.Embers.Replace(t.Context(), ember.ID, ember); err != nil {
		t.Fatal(err)
	}

	d.Remote = &activityRuntime{status: model.RuntimeStatus{ServerID: f.server.ID, State: model.RuntimeStateRunning, Busy: true}}

	proposal := ember.ProposedServerConfiguration(f.server.ServerConfiguration)
	if _, err := d.Servers.ApplyEmber(t.Context(), f.server.ID, ember.ID, proposal); !errors.Is(err, ErrServerActive) {
		t.Fatal("server-value review bypassed busy guard", err)
	}

	d.Remote = &activityRuntime{status: model.RuntimeStatus{ServerID: f.server.ID, State: model.RuntimeStateRunning}}
	input := proposal

	input.Ports = nil
	if _, err := d.Servers.ApplyEmber(t.Context(), f.server.ID, ember.ID, input); !errors.Is(err, ErrServerActive) {
		t.Fatal("running update released active ports", err)
	}
}

func TestPendingCompatibleServerValuesDoNotBlockLatestBackupMetadata(t *testing.T) {
	f := newTransferFixture(t)
	d := f.service.Deployment

	ember, err := d.Embers.Get(t.Context(), f.server.EmberID)
	if err != nil {
		t.Fatal(err)
	}

	ember.Variables = append(ember.Variables, model.Variable{Name: "NEW_OPTIONAL_VALUE", Editable: true})

	ember.SavedataPaths = []model.SavedataPath{{Path: "world.bin", Required: true}}
	if _, err := d.Embers.Replace(t.Context(), ember.ID, ember); err != nil {
		t.Fatal(err)
	}

	server, err := d.Servers.Get(t.Context(), f.server.ID)
	if err != nil || !server.EmberReviewRequired || !server.SavedataAvailable {
		t.Fatal("new compatible value bypassed review or disabled save-data backups", server, err)
	}

	if _, err := d.Sync(t.Context(), server.ID); !errors.Is(err, ErrInvalidServer) {
		t.Fatal("synchronization silently added a server value", err)
	}

	for _, mode := range []model.BackupMode{model.BackupModeFull, model.BackupModeSavedata} {
		options, err := d.backupOptions(t.Context(), server.ID, mode)
		if err != nil || options.Definition != nil || options.EmberRevision != model.EmberHash(ember) {
			t.Fatal("pending review blocked backup data or misrepresented portable configuration", options, err)
		}

		if mode == model.BackupModeSavedata && (len(options.Paths) != 1 || options.Paths[0].Path != "world.bin") {
			t.Fatal("backup did not use the latest save paths", options)
		}
	}

	proposal := ember.ProposedServerConfiguration(server.ServerConfiguration)
	if _, err := d.Servers.ApplyEmber(t.Context(), server.ID, ember.ID, proposal); err != nil {
		t.Fatal(err)
	}

	if _, err := d.Sync(t.Context(), server.ID); err != nil {
		t.Fatal("reviewed values did not become synchronizable", err)
	}
}

func TestServerReviewRejectsAnEmberChangedSinceOpeningTheWizard(t *testing.T) {
	f := newTransferFixture(t)
	d := f.service.Deployment

	ember, err := d.Embers.Get(t.Context(), f.server.EmberID)
	if err != nil {
		t.Fatal(err)
	}

	ember.Variables = append(ember.Variables, model.Variable{Name: "NEW_VALUE", Default: "one", Editable: true})

	ember, err = d.Embers.Replace(t.Context(), ember.ID, ember)
	if err != nil {
		t.Fatal(err)
	}

	expected := ember.Revision
	proposal := ember.ProposedServerConfiguration(f.server.ServerConfiguration)

	ember.Variables[0].Default = "two"
	if _, err := d.Embers.Replace(t.Context(), ember.ID, ember); err != nil {
		t.Fatal(err)
	}

	if _, err := d.Servers.ApplyEmber(t.Context(), f.server.ID, ember.ID, proposal, expected); !errors.Is(err, ErrServerConflict) {
		t.Fatal("stale review was accepted", err)
	}
}
