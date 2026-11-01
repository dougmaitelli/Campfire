//go:build !production

package devmock

import (
	"testing"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

func TestMockTransferHistoryIsSeededOnceWithoutReservingServers(t *testing.T) {
	_, db, auth, _ := fixture(t)
	transfers := repository.NewTransferRepository(db.ORM())

	items, err := transfers.List(t.Context(), "creative", false)
	if err != nil || len(items) != 2 {
		t.Fatal("missing transfer history examples", items, err)
	}

	server, err := repository.NewServerRepository(db.ORM()).Get(t.Context(), "creative")
	if err != nil {
		t.Fatal(err)
	}

	states := make(map[model.TransferPhase]bool)

	for _, item := range items {
		if item.Active || item.CanCancel || item.SourceSparkID == item.TargetSparkID || item.UpdatedAt <= item.CreatedAt {
			t.Fatal("invalid historical transfer", item)
		}

		if item.Phase == model.TransferPhaseComplete && (item.TargetSparkID != server.SparkID || item.TransferredBytes != item.TotalBytes) {
			t.Fatal("completed example does not match current placement", item)
		}

		if item.Phase == model.TransferPhaseCancelled && (item.SourceSparkID != server.SparkID || item.TransferredBytes >= item.TotalBytes) {
			t.Fatal("cancelled example changed placement or completed its copy", item)
		}

		states[item.Phase] = true
	}

	if !states[model.TransferPhaseComplete] || !states[model.TransferPhaseCancelled] {
		t.Fatal("missing transfer outcome variation", states)
	}

	if err := transfers.Guard(t.Context(), server.ID); err != nil {
		t.Fatal("historical transfers reserve the server", err)
	}

	if _, err := New(t.Context(), db.ORM(), auth); err != nil {
		t.Fatal(err)
	}

	reloaded, err := transfers.List(t.Context(), server.ID, false)
	if err != nil || len(reloaded) != len(items) {
		t.Fatal("mock restart duplicated transfer history", reloaded, err)
	}
}

func TestMockTransferHistoryDoesNotRecreateDeletedServer(t *testing.T) {
	_, db, auth, _ := fixture(t)
	if err := db.ORM().Delete(&seedMarker{ID: transferHistorySeedMarkerID}).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Where("id = ?", "creative").Delete(&database.Server{}).Error; err != nil {
		t.Fatal(err)
	}

	if _, err := New(t.Context(), db.ORM(), auth); err != nil {
		t.Fatal("adding mock history failed after deleting the example server", err)
	}
}
