//go:build !production

package devmock

import (
	"testing"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
)

func configurationServices(runtime *Runtime) *service.DeploymentService {
	embers := service.NewEmberService(repository.NewEmberRepository(runtime.db))
	servers := service.NewServerService(repository.NewServerRepository(runtime.db), embers)
	deployment := &service.DeploymentService{
		Servers: servers, Embers: embers, Sparks: service.NewSparkService(repository.NewSparkRepository(runtime.db), runtime), Remote: runtime,
	}
	servers.WithRuntimeGuard(deployment)
	deployment.History = service.NewResourceHistoryService(repository.NewResourceHistoryRepository(runtime.db), deployment)

	return deployment
}

func TestMockConfigurationStatesSurviveCollectionAndBackendRebuild(t *testing.T) {
	runtime, db, auth, _ := fixture(t)
	for range 2 {
		deployment := configurationServices(runtime)
		if err := service.NewRuntimeStatusService(deployment).Collect(t.Context()); err != nil {
			t.Fatal(err)
		}

		for _, example := range []struct {
			id      string
			state   model.RuntimeState
			sync    bool
			restart bool
		}{
			{restartExampleID, model.RuntimeStateRunning, false, true},
			{syncExampleID, model.RuntimeStateRunning, true, false},
			{firstSyncExampleID, model.RuntimeStateNotInstalled, true, false},
			{"creative", model.RuntimeStateStopped, false, false},
		} {
			server, err := deployment.Servers.Get(t.Context(), example.id)
			if err != nil || server.EmberReviewRequired || server.DeploymentSyncRequired != example.sync {
				t.Fatal("collection reset the intended configuration state", example.id, server, err)
			}

			status, err := runtime.Action(t.Context(), server.SparkID, model.RuntimeRequest{ServerID: server.ID, Action: model.ServerActionStatus})
			if err != nil || status.State != example.state || status.RestartRequired != example.restart {
				t.Fatal("collection reset the intended running baseline", example.id, status, err)
			}
		}

		var err error

		runtime, err = New(t.Context(), db.ORM(), auth)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestMockReviewRetainsCustomValuesAndAllowsBothBackupModes(t *testing.T) {
	runtime, _, _, fleet := fixture(t)
	deployment := configurationServices(runtime)

	var server model.Server

	for _, candidate := range fleet {
		if candidate.Name == mockUpdateServerName {
			server = candidate
		}
	}

	if !server.EmberReviewRequired || !server.SavedataAvailable || len(server.Ports) != 2 || server.Ports[0].ContainerPort == server.Ports[1].ContainerPort || server.Ports[1].Protocol != model.NetworkUDP {
		t.Fatal("review fixture lost its running configuration/port variations", server)
	}

	ember, err := deployment.Embers.Get(t.Context(), server.EmberID)
	if err != nil {
		t.Fatal(err)
	}

	proposal := ember.ProposedServerConfiguration(server.ServerConfiguration)
	if proposal.Variables["MOTD"] != server.Variables["MOTD"] || proposal.Image == server.Image || proposal.StartupCommand == server.StartupCommand || proposal.Variables["INSTALL_CHANNEL"] == server.Variables["INSTALL_CHANNEL"] {
		t.Fatal("review fixture lost its retained and changed values", proposal)
	}

	if value, exists := proposal.Variables["EXTRA_ARGUMENTS"]; !exists || value != "" || proposal.Variables["ENABLE_CROSSPLAY"] != "1" {
		t.Fatal("review fixture lost its optional-empty and boolean additions", proposal.Variables)
	}

	if _, exists := proposal.Variables["RETIRED_SETTING"]; exists || proposal.Variables["INTERNAL_TOKEN"] == server.Variables["INTERNAL_TOKEN"] {
		t.Fatal("review fixture lost its private and removed changes")
	}

	for _, mode := range []model.BackupMode{model.BackupModeFull, model.BackupModeSavedata} {
		status, err := deployment.Backup(t.Context(), server.ID, mode)
		if err != nil || status.State != model.RuntimeStateRunning {
			t.Fatal("pending review blocked a backup or stopped the server", mode, status, err)
		}

		var backup backupRecord
		if err := runtime.db.Take(&backup, "server_id = ? AND name = ?", server.ID, mode.ArchiveName(status.OperationID)).Error; err != nil || backup.Options.Definition != nil {
			t.Fatal("pending review claimed a portable definition", backup.Options, err)
		}
	}

	if _, err := deployment.Servers.ApplyEmber(t.Context(), server.ID, server.EmberID, proposal, ember.Revision); err != nil {
		t.Fatal(err)
	}

	if _, err := deployment.Sync(t.Context(), server.ID); err != nil {
		t.Fatal(err)
	}

	status, err := deployment.Action(t.Context(), server.ID, model.ServerActionStatus)
	if err != nil || status.State != model.RuntimeStateRunning || !status.RestartRequired {
		t.Fatal("review failed to preserve the running process and request restart", status, err)
	}
}

func TestMockConfigurationRecoveryPreservesBackupsAndPendingInstallation(t *testing.T) {
	runtime, db, auth, _ := fixture(t)

	deployment := configurationServices(runtime)
	if _, err := deployment.Sync(t.Context(), syncExampleID); err == nil {
		t.Fatal("paused configuration delivery unexpectedly succeeded")
	}

	if status, err := deployment.Backup(t.Context(), syncExampleID, model.BackupModeSavedata); err != nil || status.State != model.RuntimeStateRunning {
		t.Fatal("configuration-only failure blocked backups", status, err)
	}

	if _, err := deployment.Console(t.Context(), syncExampleID, "mock-config resume", false); err != nil {
		t.Fatal(err)
	}

	runtime, err := New(t.Context(), db.ORM(), auth)
	if err != nil {
		t.Fatal(err)
	}

	deployment = configurationServices(runtime)
	for _, id := range []string{syncExampleID, firstSyncExampleID} {
		if _, err := deployment.Sync(t.Context(), id); err != nil {
			t.Fatal("rebuild reset delivery recovery", id, err)
		}

		server, err := deployment.Servers.Get(t.Context(), id)
		if err != nil || server.DeploymentSyncRequired || server.EmberHash == "" {
			t.Fatal("recovered sync was not acknowledged", server, err)
		}
	}

	status, err := deployment.Action(t.Context(), firstSyncExampleID, model.ServerActionStatus)
	if err != nil || status.State != model.RuntimeStateNotInstalled {
		t.Fatal("configuration sync implicitly installed or started the server", status, err)
	}

	status, err = deployment.Action(t.Context(), syncExampleID, model.ServerActionStatus)
	if err != nil || !status.RestartRequired || status.State != model.RuntimeStateRunning {
		t.Fatal("recovered delivery lost the old running baseline", status, err)
	}
}

func TestMockMetadataOnlyPathsApplyWithoutSynchronization(t *testing.T) {
	runtime, _, _, _ := fixture(t)
	deployment := configurationServices(runtime)

	server, err := deployment.Servers.Get(t.Context(), "creative")
	if err != nil || server.EmberUpdateAvailable || !server.SavedataAvailable {
		t.Fatal("metadata-only fixture requested review or synchronization", server, err)
	}

	status, err := deployment.Backup(t.Context(), server.ID, model.BackupModeSavedata)
	if err != nil {
		t.Fatal(err)
	}

	var backup backupRecord
	if err := runtime.db.Take(&backup, "server_id = ? AND name = ?", server.ID, model.BackupModeSavedata.ArchiveName(status.OperationID)).Error; err != nil {
		t.Fatal(err)
	}

	if _, exists := backup.Files["whitelist.json"]; !exists {
		t.Fatal("backup did not use the latest metadata-only path change", backup.Files)
	}
}

func TestMockBackupRestartAppliesSavedContainerConfiguration(t *testing.T) {
	runtime, db, auth, _ := fixture(t)

	status, err := runtime.Action(t.Context(), "mock-europe", model.RuntimeRequest{
		ServerID: restartExampleID, Action: model.ServerActionBackup, OperationID: "backup-restart", BackupOptions: model.BackupOptions{StopServer: true},
	})
	if err != nil || status.State != model.RuntimeStateRunning || status.RestartRequired {
		t.Fatal("backup restart retained an obsolete container baseline", status, err)
	}

	runtime, err = New(t.Context(), db.ORM(), auth)
	if err != nil {
		t.Fatal(err)
	}

	status, err = runtime.Action(t.Context(), "mock-europe", model.RuntimeRequest{ServerID: restartExampleID, Action: model.ServerActionStatus})
	if err != nil || status.RestartRequired {
		t.Fatal("backend rebuild lost the applied restart", status, err)
	}
}

func TestMockFirstSyncAllowsQueuingTransferBeforeAcknowledgement(t *testing.T) {
	runtime, db, _, _ := fixture(t)
	transfers := mockTransferService(t, runtime, db)

	server, err := transfers.Deployment.Servers.Get(t.Context(), firstSyncExampleID)
	if err != nil || server.EmberHash != "" || !server.DeploymentSyncRequired {
		t.Fatal("fixture already acknowledged its first configuration", server, err)
	}

	work, err := transfers.Begin(t.Context(), server.ID, "mock-europe", []model.ServerPort{
		{BindIP: "0.0.0.0", HostPort: 25584, ContainerPort: 25565, Protocol: model.NetworkTCP},
	})
	if err != nil || !work.Active {
		t.Fatal("first-sync fixture cannot queue a transfer", work, err)
	}
}
