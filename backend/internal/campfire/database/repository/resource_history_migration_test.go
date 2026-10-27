package repository_test

import (
	"testing"

	"campfire/internal/campfire/database"
)

func TestResourceHistoryUpgradeRemovesDiskRollups(t *testing.T) {
	source := testDatabaseSource(t)

	db, err := database.Open(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}

	for _, table := range []string{"server_resource_buckets", "spark_resource_buckets"} {
		for _, column := range []string{"disk_count", "disk_sum", "disk_peak", "disk_limit_sum", "disk_unlimited"} {
			if err := db.ORM().Exec("ALTER TABLE " + table + " ADD COLUMN " + column + " BIGINT NOT NULL DEFAULT 0").Error; err != nil {
				t.Fatal(err)
			}
		}
	}

	if err := db.ORM().Create(&database.Spark{ID: "node", Name: "Node"}).Error; err != nil {
		t.Fatal(err)
	}

	bucket := database.SparkResourceBucket{SparkID: "node", ResourceAggregate: database.ResourceAggregate{BucketStart: 300, SampleCount: 3, CPUCount: 3, CPUSum: 150, MemoryCount: 3, MemorySum: 3072}}
	if err := db.ORM().Omit("Spark").Create(&bucket).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = database.Open(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	for _, table := range []string{"server_resource_buckets", "spark_resource_buckets"} {
		for _, column := range []string{"disk_count", "disk_sum", "disk_peak", "disk_limit_sum", "disk_unlimited"} {
			if db.ORM().Migrator().HasColumn(table, column) {
				t.Fatal("obsolete disk history column survived upgrade", table, column)
			}
		}
	}

	var preserved database.SparkResourceBucket
	if err := db.ORM().First(&preserved).Error; err != nil || preserved.CPUSum != 150 || preserved.MemorySum != 3072 {
		t.Fatal("CPU or memory history lost during upgrade", preserved, err)
	}

	bucket.BucketStart = 600
	if err := db.ORM().Omit("Spark").Create(&bucket).Error; err != nil {
		t.Fatal("upgraded rollups cannot accept new measurements", err)
	}
}
