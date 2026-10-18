//go:build !production

package devmock

import (
	"testing"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
)

func TestMockServerCanApplyImportedEmberUpdateAndKeepItAfterRestart(t *testing.T) {
	runtime, db, auth, fleet := fixture(t)

	var server model.Server

	for _, item := range fleet {
		if item.Name == mockUpdateServerName {
			server = item
		}
	}

	if server.ID == "" || server.EmberID != "mock-minecraft" {
		t.Fatal("missing server pinned to the old Ember", server)
	}

	embers := service.NewEmberService(repository.NewEmberRepository(db.ORM()))

	updated, err := embers.Get(t.Context(), mockImportedUpdateID)
	if err != nil || updated.UpdateURL != mockPendingUpdateURL || updated.Installer == nil {
		t.Fatal("missing imported update", updated, err)
	}

	status, err := runtime.Action(t.Context(), server.SparkID, model.RuntimeRequest{ServerID: server.ID, Action: "status"})
	if err != nil || status.State != "stopped" {
		t.Fatal("update fixture is not ready for an explicit Ember update", status, err)
	}

	store := repository.NewServerRepository(db.ORM())
	servers := service.NewServerService(store, embers)
	deployment := &service.DeploymentService{Servers: servers, Sparks: service.NewSparkService(repository.NewSparkRepository(db.ORM()), runtime), Embers: embers, Remote: runtime}
	servers.WithRuntimeGuard(deployment)

	input := server.ServerConfiguration
	input.EmberID = updated.ID

	saved, err := servers.ApplyEmber(t.Context(), server.ID, server.EmberID, input)
	if err != nil || saved.EmberID != updated.ID {
		t.Fatal("could not apply imported Ember update", saved, err)
	}

	if _, err := New(t.Context(), db.ORM(), auth); err != nil {
		t.Fatal(err)
	}

	saved, err = store.Get(t.Context(), server.ID)
	if err != nil || saved.EmberID != updated.ID {
		t.Fatal("mock restart reset the applied Ember revision", saved, err)
	}

	status, err = runtime.Action(t.Context(), server.SparkID, model.RuntimeRequest{ServerID: server.ID, Action: "status"})
	if err != nil || status.State != "stopped" {
		t.Fatal("applying an Ember unexpectedly started the server", status, err)
	}
}

func TestMockPendingUpdatesDownloadAndImportWithoutNetwork(t *testing.T) {
	runtime, db, auth, _ := fixture(t)
	embers := service.NewEmberService(repository.NewEmberRepository(db.ORM()))
	checks := repository.NewEmberUpdateRepository(db.ORM())
	updates := service.NewEmberUpdateService(embers, checks).WithFetcher(runtime.FetchEmberUpdate)

	seeded, err := checks.List(t.Context())
	if err != nil || len(seeded) != 2 {
		t.Fatal("missing mock update states", seeded, err)
	}

	item, preview, err := updates.Check(t.Context(), "mock-minecraft")
	if err != nil || !item.Available || preview.Ember == nil || preview.Ember.Installer == nil {
		t.Fatal("pending update could not be downloaded", item, preview, err)
	}

	current, _, err := updates.Check(t.Context(), "mock-current")
	if err != nil || current.Available || current.Error != "" {
		t.Fatal("current fixture incorrectly has an update", current, err)
	}

	imported, err := embers.ImportDefinition(t.Context(), []byte(preview.Definition), service.ImportOptions{SourceURL: mockPendingUpdateURL, AcknowledgeWarnings: true, ReplaceID: "mock-minecraft"})
	if err != nil || imported.ID != "mock-minecraft" || imported.UpdateURL != mockPendingUpdateURL {
		t.Fatal("mock update did not replace the local Ember", imported, err)
	}

	updated, _, err := updates.Check(t.Context(), imported.ID)
	if err != nil || updated.Available || updated.Error != "" {
		t.Fatal("downloaded revision is not current", updated, err)
	}

	if _, err := New(t.Context(), db.ORM(), auth); err != nil {
		t.Fatal(err)
	}

	items, err := embers.List(t.Context())
	if err != nil || len(items) != 2 {
		t.Fatal("mock restart changed imported revisions", items, err)
	}
}
