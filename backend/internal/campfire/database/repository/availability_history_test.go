package repository_test

import (
	"reflect"
	"testing"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

func TestAvailabilitySplitsBucketsAndSurvivesMetricRollup(t *testing.T) {
	db, store := resourceHistoryStore(t)

	const base int64 = 1800000000

	spans := []repository.AvailabilitySpan{
		{ServerID: "game", SparkID: "node", From: base + 295, To: base + 305, Online: true},
		{ServerID: "game", SparkID: "node", From: base + 305, To: base + 315},
	}
	if err := store.RecordAvailability(t.Context(), spans); err != nil {
		t.Fatal(err)
	}
	// Old running metrics alone must not invent known availability. Resource
	// rollup and availability update disjoint columns, in either write order.
	saveResourcePoint(t, store, base+10, new(float64(50)), new(int64(100)), nil)
	saveResourcePoint(t, store, base+620, new(float64(80)), new(int64(100)), nil)

	read := func() model.AvailabilityHistory {
		t.Helper()

		result, err := store.Availability(t.Context(), []string{"game"}, base+900)
		if err != nil {
			t.Fatal(err)
		}

		return result["game"]
	}

	before := read()
	if before.From != base+900-86400 || before.To != base+900 || len(before.Points) != 288 {
		t.Fatal(before)
	}

	if before.Points[285] != (model.AvailabilityPoint{OnlineSeconds: 5}) || before.Points[286] != (model.AvailabilityPoint{OnlineSeconds: 5, OfflineSeconds: 10}) || before.Points[287] != (model.AvailabilityPoint{}) {
		t.Fatal(before.Points[285:])
	}

	for range 2 {
		if err := store.Rollup(t.Context(), base+900); err != nil {
			t.Fatal(err)
		}

		if got := read(); !reflect.DeepEqual(before, got) {
			t.Fatal("metric compaction changed availability")
		}
	}

	if err := store.RecordAvailability(t.Context(), []repository.AvailabilitySpan{{ServerID: "game", SparkID: "node", From: base + 280, To: base + 290, Online: true}}); err != nil {
		t.Fatal(err)
	}

	var row database.ServerResourceBucket
	if err := db.ORM().Take(&row, "server_id = ? AND bucket_start = ?", "game", base).Error; err != nil {
		t.Fatal(err)
	}

	if row.SampleCount != 1 || row.CPUCount != 1 || row.CPUSum != 50 || row.OnlineSeconds != 15 {
		t.Fatal("availability changed resource aggregates", row)
	}

	hidden, err := store.Availability(t.Context(), []string{"other"}, base+900)
	if err != nil || len(hidden) != 1 || !reflect.DeepEqual(hidden["other"], model.EmptyAvailabilityHistory(base+900)) {
		t.Fatal(hidden, err)
	}

	if err := store.Prune(t.Context(), base+900); err != nil {
		t.Fatal(err)
	}

	if got := read(); !reflect.DeepEqual(got, model.EmptyAvailabilityHistory(base+900)) {
		t.Fatal("expired history retained")
	}
}

func TestAvailabilityRejectsInvalidSpansAndFencesPlacement(t *testing.T) {
	_, store := resourceHistoryStore(t)

	const base int64 = 1800000000
	for _, span := range []repository.AvailabilitySpan{
		{From: base, To: base}, {From: base, To: base - 1}, {From: base, To: base + 301}, {From: -1, To: 0},
	} {
		if err := store.RecordAvailability(t.Context(), []repository.AvailabilitySpan{span}); err == nil {
			t.Fatal("invalid span accepted", span)
		}
	}

	if err := store.RecordAvailability(t.Context(), []repository.AvailabilitySpan{
		{ServerID: "game", SparkID: "old-node", From: base, To: base + 10, Online: true},
		{ServerID: "deleted", SparkID: "node", From: base, To: base + 10, Online: true},
	}); err != nil {
		t.Fatal(err)
	}

	got, err := store.Availability(t.Context(), []string{"game", "deleted"}, base+300)
	if err != nil {
		t.Fatal(err)
	}

	for _, history := range got {
		if !reflect.DeepEqual(history, model.EmptyAvailabilityHistory(base+300)) {
			t.Fatal(history)
		}
	}

	if _, err := store.Availability(t.Context(), make([]string, 51), base); err == nil {
		t.Fatal("unbounded targets")
	}
}

func TestAvailabilityClampsCoverageAndExcludesCurrentBucket(t *testing.T) {
	db, store := resourceHistoryStore(t)

	const base int64 = 1800000000

	rows := []database.ServerResourceBucket{
		{ServerID: "game", ResourceAggregate: database.ResourceAggregate{BucketStart: base, OnlineSeconds: 300, OfflineSeconds: 120}},
		{ServerID: "game", ResourceAggregate: database.ResourceAggregate{BucketStart: base + 300, OfflineSeconds: 300}},
	}
	if err := db.ORM().Omit("Server").Create(&rows).Error; err != nil {
		t.Fatal(err)
	}

	got, err := store.Availability(t.Context(), []string{"game"}, base+350)
	if err != nil {
		t.Fatal(err)
	}

	if last := got["game"].Points[287]; last.OnlineSeconds != 180 || last.OfflineSeconds != 120 {
		t.Fatal(last)
	}
}

func TestSchemaSevenAddsUnknownAvailabilityWithoutInventingHistory(t *testing.T) {
	source := testDatabaseSource(t)

	db, err := database.Open(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}

	for _, row := range []any{&database.ServerResourceBucket{}, &database.SparkResourceBucket{}} {
		for _, column := range []string{"online_seconds", "offline_seconds"} {
			if err := db.ORM().Migrator().DropColumn(row, column); err != nil {
				t.Fatal(err)
			}
		}
	}

	if err := db.ORM().Exec("UPDATE schema_migrations SET version = 7").Error; err != nil {
		t.Fatal(err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	for range 2 {
		db, err = database.Open(t.Context(), source)
		if err != nil {
			t.Fatal(err)
		}

		for _, row := range []any{&database.ServerResourceBucket{}, &database.SparkResourceBucket{}} {
			for _, column := range []string{"online_seconds", "offline_seconds"} {
				if !db.ORM().Migrator().HasColumn(row, column) {
					t.Fatal("missing availability column", column)
				}
			}
		}

		store := repository.NewResourceHistoryRepository(db.ORM())

		result, err := store.Availability(t.Context(), []string{"game"}, 1800000000)
		if err != nil || !reflect.DeepEqual(result["game"], model.EmptyAvailabilityHistory(1800000000)) {
			t.Fatal(result, err)
		}

		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
