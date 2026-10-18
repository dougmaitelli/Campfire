//go:build !production

package devmock

import (
	"testing"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
)

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

	imported, err := embers.ImportDefinition(t.Context(), []byte(preview.Definition), service.ImportOptions{SourceURL: mockPendingUpdateURL, AcknowledgeWarnings: true})
	if err != nil || imported.ID == "mock-minecraft" || imported.UpdateURL != mockPendingUpdateURL {
		t.Fatal("mock update did not create a new revision", imported, err)
	}

	updated, _, err := updates.Check(t.Context(), imported.ID)
	if err != nil || updated.Available || updated.Error != "" {
		t.Fatal("downloaded revision is not current", updated, err)
	}

	if _, err := New(t.Context(), db.ORM(), auth); err != nil {
		t.Fatal(err)
	}

	items, err := embers.List(t.Context())
	if err != nil || len(items) != 3 {
		t.Fatal("mock restart changed imported revisions", items, err)
	}
}
