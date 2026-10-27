package repository_test

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

func resourceHistoryStore(t *testing.T) (*database.Database, *repository.ResourceHistoryRepository) {
	t.Helper()

	db, _ := historyStore(t)
	for _, row := range []any{
		&database.Spark{ID: "node", Name: "Node"},
		&database.Ember{ID: "ember", Name: "Game", Definition: model.Ember{Schema: 1}},
		&database.Server{ID: "game", Name: "Game", SparkID: "node", EmberID: "ember", Image: "image", GameIntegration: true, Variables: map[string]string{}, Ports: []model.ServerPort{}},
	} {
		if err := db.ORM().Omit("Spark", "Ember", "Owner").Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}

	return db, repository.NewResourceHistoryRepository(db.ORM())
}

func TestResourceHistoryMonthQueryIsBounded(t *testing.T) {
	db, store := resourceHistoryStore(t)

	const bucketCount = 30 * 24 * 12

	base := time.Now().Truncate(5 * time.Minute).Add(-30 * 24 * time.Hour).Unix()

	rows := make([]database.ServerResourceBucket, bucketCount)
	for i := range rows {
		start := base + int64(i)*repository.ResourceBucketSeconds
		rows[i] = database.ServerResourceBucket{ServerID: "game", ResourceAggregate: database.ResourceAggregate{BucketStart: start, SampleCount: 30, CPUCount: 30, CPUSum: 1500, CPUPeak: 100, CPULimitSum: 60, MemoryUnlimited: 30, LatestAt: start + 290}}
	}

	if err := db.ORM().Omit("Server").CreateInBatches(&rows, 100).Error; err != nil {
		t.Fatal(err)
	}

	points, err := store.History(t.Context(), false, "game", base, base+30*86400, 3*3600, 240)
	if err != nil || len(points) != 240 {
		t.Fatal("month query did not honor point bound", len(points), err)
	}

	var samples int64
	for _, point := range points {
		samples += point.SampleCount
		if point.Usage.CPUPercent == nil || *point.Usage.CPUPercent != 50 || point.CPUPeak == nil || *point.CPUPeak != 100 {
			t.Fatal("month query lost weighted values or peaks", point)
		}
	}

	if samples != bucketCount*30 {
		t.Fatal("month query lost samples", samples)
	}
}

func TestResourceHistoryBacklogDoesNotTriggerUnboundedReads(t *testing.T) {
	db, store := resourceHistoryStore(t)
	base := time.Now().Truncate(5 * time.Minute).Add(-24 * time.Hour).Unix()

	rows := make([]database.ServerResourceSample, 2050)
	for i := range rows {
		at := base + int64(i)*10
		rows[i] = database.ServerResourceSample{ServerID: "game", SparkID: "node", CollectedAt: at, ReceivedAt: at, CPULimit: 2, Usage: model.ResourceUsage{ServerID: "game", State: "running", CPUPercent: new(float64(25)), SampledAt: at}}
	}

	if err := db.ORM().Omit("Server").CreateInBatches(&rows, 100).Error; err != nil {
		t.Fatal(err)
	}

	if _, err := store.History(t.Context(), false, "game", base, base+86400, 3600, 24); !errors.Is(err, repository.ErrResourceHistoryPending) {
		t.Fatal("request tried to scan uncompacted backlog", err)
	}

	if err := store.Rollup(t.Context(), base+86400); err != nil {
		t.Fatal(err)
	}

	points, err := store.History(t.Context(), false, "game", base, base+86400, 3600, 24)
	if err != nil || len(points) == 0 || len(points) > 24 {
		t.Fatal("rollup job did not recover the backlog", points, err)
	}
}

func saveResourcePoint(t *testing.T, store *repository.ResourceHistoryRepository, at int64, cpu *float64, memory, disk *int64) {
	t.Helper()

	usage := model.ResourceUsage{ServerID: "game", State: "running", CPUPercent: cpu, MemoryBytes: memory, DiskBytes: disk, SampledAt: at, DiskSampledAt: at}
	server := database.ServerResourceSample{ServerID: "game", SparkID: "node", CollectedAt: at, ReceivedAt: at, CPULimit: 2, MemoryMB: 4096, DiskMB: 10240, Usage: usage}
	usage.ServerID = ""

	node := database.SparkResourceSample{SparkID: "node", CollectedAt: at, ReceivedAt: at, CPULimit: 4, MemoryMB: 8192, DiskMB: 20480, Usage: usage}
	if err := store.Save(t.Context(), []database.ServerResourceSample{server}, []database.SparkResourceSample{node}); err != nil {
		t.Fatal(err)
	}
}

func TestResourceRollupWeightedAveragesPeaksAndRawTail(t *testing.T) {
	db, store := resourceHistoryStore(t)
	base := time.Now().Truncate(5 * time.Minute).Add(-30 * time.Minute).Unix()
	saveResourcePoint(t, store, base+10, new(float64(10)), new(int64(100)), new(int64(1)))
	saveResourcePoint(t, store, base+20, new(float64(20)), new(int64(200)), new(int64(2)))
	saveResourcePoint(t, store, base+30, new(float64(100)), new(int64(300)), nil)
	saveResourcePoint(t, store, base+310, new(float64(50)), nil, new(int64(3)))
	saveResourcePoint(t, store, base+620, nil, nil, nil)
	saveResourcePoint(t, store, base+1810, new(float64(90)), new(int64(900)), new(int64(9)))

	before, err := store.History(t.Context(), false, "game", base, base+1900, 600, 10)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.Rollup(t.Context(), base+1820); err != nil {
		t.Fatal(err)
	}

	after, err := store.History(t.Context(), false, "game", base, base+1900, 600, 10)
	if err != nil || !reflect.DeepEqual(before, after) {
		a, _ := json.Marshal(before)
		b, _ := json.Marshal(after)
		t.Fatalf("rollup changed history: %s -> %s, %v", a, b, err)
	}

	if len(after) != 3 || after[0].SampleCount != 4 || after[0].CPUSamples != 4 || math.Abs(*after[0].Usage.CPUPercent-45) > 0.001 || *after[0].CPUPeak != 100 || after[0].MemorySamples != 3 || *after[0].Usage.MemoryBytes != 200 {
		t.Fatal("incorrect weighted averages or peaks", after)
	}

	if after[1].Usage.CPUPercent != nil || after[1].Usage.MemoryBytes != nil || after[1].SampleCount != 1 {
		t.Fatal("missing samples became zeros", after[1])
	}

	latest, err := store.Latest(t.Context(), []string{"game"})
	if err != nil || latest["game"].CollectedAt != base+1810 || *latest["game"].Usage.CPUPercent != 90 || *latest["game"].Usage.DiskBytes != 9 {
		t.Fatal("latest exact sample was lost", latest, err)
	}

	var count int64
	if err := db.ORM().Model(&database.ServerResourceSample{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("raw samples were not compacted", count, err)
	}

	if err := store.Rollup(t.Context(), base+1820); err != nil {
		t.Fatal(err)
	}

	repeated, err := store.History(t.Context(), false, "game", base, base+1900, 600, 10)
	if err != nil || !reflect.DeepEqual(after, repeated) {
		t.Fatal("repeated rollup double counted history", err)
	}

	saveResourcePoint(t, store, base+40, new(float64(80)), nil, nil)

	if err := store.Rollup(t.Context(), base+1820); err != nil {
		t.Fatal(err)
	}

	late, err := store.History(t.Context(), true, "node", base, base+1900, 600, 10)
	if err != nil || late[0].SampleCount != 5 || math.Abs(*late[0].Usage.CPUPercent-52) > 0.001 {
		t.Fatal("late sample was not merged exactly once", late, err)
	}
}

func TestResourceHistoryRetentionIndexesAndCascade(t *testing.T) {
	db, store := resourceHistoryStore(t)
	base := time.Now().Truncate(5 * time.Minute).Add(-time.Hour).Unix()
	saveResourcePoint(t, store, base+10, new(float64(10)), nil, nil)
	saveResourcePoint(t, store, base+610, new(float64(20)), nil, nil)
	saveResourcePoint(t, store, base+1210, new(float64(30)), nil, nil)

	if err := store.Rollup(t.Context(), base+1220); err != nil {
		t.Fatal(err)
	}

	if err := store.Prune(t.Context(), base+600); err != nil {
		t.Fatal(err)
	}

	points, err := store.History(t.Context(), false, "game", base, base+1300, 300, 10)
	if err != nil || len(points) != 2 || points[0].CollectedAt != base+600 {
		t.Fatal("retention removed the wrong history", points, err)
	}

	for _, table := range []struct {
		model          any
		resource, time string
	}{
		{&database.ServerResourceSample{}, "server_id", "collected_at"},
		{&database.SparkResourceSample{}, "spark_id", "collected_at"},
		{&database.ServerResourceBucket{}, "server_id", "bucket_start"},
		{&database.SparkResourceBucket{}, "spark_id", "bucket_start"},
	} {
		indexes, err := db.ORM().Migrator().GetIndexes(table.model)
		if err != nil {
			t.Fatal(err)
		}

		composite, timeIndex := false, false

		var definitions [][]string

		if db.ORM().Name() == "postgres" {
			// GORM deliberately excludes constraint-backed PostgreSQL indexes.
			name := db.ORM().NamingStrategy.TableName(reflect.TypeOf(table.model).Elem().Name())

			var primary []string
			if err := db.ORM().Raw(`SELECT keys.column_name FROM information_schema.table_constraints constraints
JOIN information_schema.key_column_usage keys ON keys.constraint_name = constraints.constraint_name AND keys.table_schema = constraints.table_schema AND keys.table_name = constraints.table_name
WHERE constraints.constraint_type = 'PRIMARY KEY' AND constraints.table_schema = current_schema() AND constraints.table_name = ? ORDER BY keys.ordinal_position`, name).Scan(&primary).Error; err != nil {
				t.Fatal(err)
			}

			definitions = append(definitions, primary)
			composite = reflect.DeepEqual(primary, []string{table.resource, table.time})
		}

		for _, index := range indexes {
			columns := index.Columns()
			definitions = append(definitions, columns)

			if reflect.DeepEqual(columns, []string{table.resource, table.time}) {
				composite = true
			}

			if reflect.DeepEqual(columns, []string{table.time}) {
				timeIndex = true
			}
		}

		if !composite || !timeIndex {
			t.Fatalf("missing resource/time or retention index on %T: %v", table.model, definitions)
		}
	}

	if err := db.ORM().Delete(&database.Server{ID: "game"}).Error; err != nil {
		t.Fatal(err)
	}

	for _, table := range []any{&database.ServerResourceSample{}, &database.ServerResourceBucket{}} {
		var count int64
		if err := db.ORM().Model(table).Count(&count).Error; err != nil || count != 0 {
			t.Fatal("deleted resource retained history", count, err)
		}
	}
}

func TestResourceRollupKeepsLatestAndRollsBackAtomically(t *testing.T) {
	db, store := resourceHistoryStore(t)
	base := time.Now().Truncate(5 * time.Minute).Add(-time.Hour).Unix()
	saveResourcePoint(t, store, base+10, new(float64(10)), nil, nil)
	saveResourcePoint(t, store, base+20, new(float64(20)), nil, nil)

	trigger := "CREATE TRIGGER fail_resource_compaction BEFORE DELETE ON server_resource_samples BEGIN SELECT RAISE(ABORT, 'test rollback'); END"
	dropTrigger := "DROP TRIGGER fail_resource_compaction"

	if db.ORM().Name() == "postgres" {
		if err := db.ORM().Exec("CREATE FUNCTION resource_compaction_failure() RETURNS trigger AS $$ BEGIN RAISE EXCEPTION 'test rollback'; END; $$ LANGUAGE plpgsql").Error; err != nil {
			t.Fatal(err)
		}

		trigger = "CREATE TRIGGER fail_resource_compaction BEFORE DELETE ON server_resource_samples FOR EACH ROW EXECUTE FUNCTION resource_compaction_failure()"
		dropTrigger += " ON server_resource_samples"
	}

	if err := db.ORM().Exec(trigger).Error; err != nil {
		t.Fatal(err)
	}

	if err := store.Rollup(t.Context(), base+1000); err == nil {
		t.Fatal("failed compaction committed")
	}

	var count int64
	if err := db.ORM().Model(&database.ServerResourceBucket{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("rollup survived a failed delete", count, err)
	}

	if err := db.ORM().Exec(dropTrigger).Error; err != nil {
		t.Fatal(err)
	}

	if err := store.Rollup(t.Context(), base+1000); err != nil {
		t.Fatal(err)
	}

	latest, err := store.Latest(t.Context(), []string{"game"})
	if err != nil || latest["game"].CollectedAt != base+20 {
		t.Fatal("latest old sample was deleted", latest, err)
	}

	points, err := store.History(t.Context(), false, "game", base, base+1000, 300, 10)
	if err != nil || len(points) != 1 || points[0].SampleCount != 2 || *points[0].Usage.CPUPercent != 15 {
		t.Fatal("retained latest row was double counted", points, err)
	}
}
