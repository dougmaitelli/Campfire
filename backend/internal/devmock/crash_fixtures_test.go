//go:build !production

package devmock

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

func TestCrashFixturesAppearInActivityWithPermissionGatedEvidence(t *testing.T) {
	_, db, _, _ := fixture(t)

	var records []database.ServerCrash
	if err := db.ORM().Order("exited_at DESC").Find(&records).Error; err != nil || len(records) != 4 {
		t.Fatal(records, err)
	}

	activity := repository.NewServerActivityRepository(db.ORM())

	page, err := activity.List(t.Context(), "survival", repository.ServerActivityAccess{}, repository.ServerActivityFilter{Kind: "alert", Search: "Server crashed"})
	if err != nil || len(page.Items) != 4 {
		t.Fatal(page, err)
	}

	confirmed, unknown, unavailable, truncated := 0, 0, 0, 0

	for _, saved := range records {
		record := saved.Record
		if err := record.Validate(); err != nil {
			t.Fatal(err)
		}

		if saved.ServerID != "survival" || saved.SparkID != "mock-europe" {
			t.Fatal(saved)
		}

		switch record.Reason {
		case model.CrashReasonUnknown:
			unknown++
		case model.CrashReasonOutOfMemory:
			confirmed++
		}

		if record.ConsoleUnavailable {
			unavailable++
		}

		if record.ConsoleTruncated {
			truncated++
		}

		for _, console := range []bool{false, true} {
			details, err := activity.Details(t.Context(), "survival", "crash:"+record.ID, repository.ServerActivityAccess{Console: console})
			if err != nil || details.Crash == nil {
				t.Fatal(details, err)
			}

			expected := ""
			if console {
				expected = record.Console
			}

			if details.Crash.Console != expected {
				t.Fatal("incorrect evidence access", details.Crash)
			}
		}

		if record.Reason == model.CrashReasonUnknown && strings.Contains(saved.Title, "memory") {
			t.Fatal("unknown OOM was presented as confirmed", saved)
		}
	}

	if confirmed != 1 || unknown != 3 || unavailable != 1 || truncated != 1 {
		t.Fatal(confirmed, unknown, unavailable, truncated)
	}
}

func TestCrashFixtureUpgradeAndRestartPreserveRuntimeAndRetention(t *testing.T) {
	_, db, auth, _ := fixture(t)
	// Model an existing mock database created before crash fixtures were added.
	if err := db.ORM().Where("server_id = ?", "survival").Delete(&database.ServerCrash{}).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Delete(&seedMarker{ID: crashSeedMarkerID}).Error; err != nil {
		t.Fatal(err)
	}

	var before runtimeRecord
	if err := db.ORM().Take(&before, "server_id = ?", "survival").Error; err != nil {
		t.Fatal(err)
	}

	if _, err := New(t.Context(), db.ORM(), auth); err != nil {
		t.Fatal(err)
	}

	var original []database.ServerCrash
	if err := db.ORM().Order("id").Find(&original).Error; err != nil || len(original) != 4 {
		t.Fatal(original, err)
	}

	if _, err := New(t.Context(), db.ORM(), auth); err != nil {
		t.Fatal(err)
	}

	var restored []database.ServerCrash
	if err := db.ORM().Order("id").Find(&restored).Error; err != nil || !reflect.DeepEqual(original, restored) {
		t.Fatal("restart changed crash records", restored, err)
	}

	var after runtimeRecord
	if err := db.ORM().Take(&after, "server_id = ?", "survival").Error; err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("seeding changed simulated runtime", err)
	}

	if err := db.ORM().Model(&database.ServerCrash{}).Where("id = ?", original[0].ID).Update("exited_at", time.Now().Add(-model.CrashRetention-time.Hour).Unix()).Error; err != nil {
		t.Fatal(err)
	}

	if err := repository.NewCrashRepository(db.ORM()).Prune(t.Context()); err != nil {
		t.Fatal(err)
	}

	if _, err := New(t.Context(), db.ORM(), auth); err != nil {
		t.Fatal(err)
	}

	var count int64
	if err := db.ORM().Model(&database.ServerCrash{}).Count(&count).Error; err != nil || count != 3 {
		t.Fatal("restart resurrected expired evidence", count, err)
	}
}
