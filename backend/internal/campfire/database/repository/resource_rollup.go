package repository

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ResourceBucketSeconds is the retained resolution after raw history is compacted.
const ResourceBucketSeconds int64 = 300

// ErrResourceHistoryPending prevents requests from scanning a rollup backlog.
var ErrResourceHistoryPending = errors.New("resource history compaction is catching up; retry shortly")

type resourceTable struct{ raw, bucket, key string }

var resourceTables = []resourceTable{
	{"server_resource_samples", "server_resource_buckets", "server_id"},
	{"spark_resource_samples", "spark_resource_buckets", "spark_id"},
}

type resourceRawRow struct {
	ResourceID  string
	CollectedAt int64
	ReceivedAt  int64
	CPULimit    float64
	MemoryMB    int64
	DiskMB      int64
	Usage       model.ResourceUsage `gorm:"serializer:json"`
}

func resourceRawQuery(tx *gorm.DB, table resourceTable) *gorm.DB {
	return tx.Table(table.raw).Select(table.key + " AS resource_id, collected_at, received_at, cpu_limit, memory_mb, disk_mb, usage")
}

func aggregateRaw(row resourceRawRow, start int64) database.ResourceAggregate {
	result := database.ResourceAggregate{BucketStart: start, SampleCount: 1, CPULimitSum: row.CPULimit, MemoryLimitSum: float64(row.MemoryMB), DiskLimitSum: float64(row.DiskMB), LatestAt: row.CollectedAt}
	if row.CPULimit == 0 {
		result.CPUUnlimited = 1
	}

	if row.MemoryMB == 0 {
		result.MemoryUnlimited = 1
	}

	if row.DiskMB == 0 {
		result.DiskUnlimited = 1
	}

	if row.Usage.ContainerError == "" {
		if value := row.Usage.CPUPercent; value != nil {
			result.CPUCount, result.CPUSum, result.CPUPeak = 1, *value, *value
		}

		if value := row.Usage.MemoryBytes; value != nil {
			result.MemoryCount, result.MemorySum, result.MemoryPeak = 1, float64(*value), *value
		}
	}

	if value := row.Usage.DiskBytes; value != nil && row.Usage.DiskError == "" {
		result.DiskCount, result.DiskSum, result.DiskPeak = 1, float64(*value), *value
	}

	return result
}

func mergeResourceAggregate(target *database.ResourceAggregate, value database.ResourceAggregate) {
	target.SampleCount += value.SampleCount
	target.CPUCount += value.CPUCount
	target.CPUSum += value.CPUSum
	target.CPUPeak = max(target.CPUPeak, value.CPUPeak)
	target.MemoryCount += value.MemoryCount
	target.MemorySum += value.MemorySum
	target.MemoryPeak = max(target.MemoryPeak, value.MemoryPeak)
	target.DiskCount += value.DiskCount
	target.DiskSum += value.DiskSum
	target.DiskPeak = max(target.DiskPeak, value.DiskPeak)
	target.CPULimitSum += value.CPULimitSum
	target.MemoryLimitSum += value.MemoryLimitSum
	target.DiskLimitSum += value.DiskLimitSum
	target.CPUUnlimited += value.CPUUnlimited
	target.MemoryUnlimited += value.MemoryUnlimited
	target.DiskUnlimited += value.DiskUnlimited
	target.LatestAt = max(target.LatestAt, value.LatestAt)
}

var resourceSumColumns = []string{"sample_count", "cpu_count", "cpu_sum", "memory_count", "memory_sum", "disk_count", "disk_sum", "cpu_limit_sum", "memory_limit_sum", "disk_limit_sum", "cpu_unlimited", "memory_unlimited", "disk_unlimited"}
var resourceMaxColumns = []string{"cpu_peak", "memory_peak", "disk_peak", "latest_at"}

// Rollup compacts closed buckets transactionally in bounded batches. Keeping the
// newest raw row per resource lets latest reads remain exact across restarts.
func (r *ResourceHistoryRepository) Rollup(ctx context.Context, now int64) error {
	cutoff := now / ResourceBucketSeconds * ResourceBucketSeconds

	for _, table := range resourceTables {
		for {
			processed := 0

			err := database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
				var rows []resourceRawRow

				err := resourceRawQuery(tx, table).
					Where("collected_at < ? AND collected_at < (SELECT MAX(newest.collected_at) FROM "+table.raw+" newest WHERE newest."+table.key+" = "+table.raw+"."+table.key+")", cutoff).
					Order("collected_at").Order(table.key).Limit(200).Find(&rows).Error
				if err != nil || len(rows) == 0 {
					return err
				}

				type bucketKey struct {
					id    string
					start int64
				}

				buckets := make(map[bucketKey]*database.ResourceAggregate)

				pairs := make([][]any, 0, len(rows))
				for _, row := range rows {
					key := bucketKey{row.ResourceID, row.CollectedAt / ResourceBucketSeconds * ResourceBucketSeconds}
					if buckets[key] == nil {
						buckets[key] = &database.ResourceAggregate{BucketStart: key.start}
					}

					mergeResourceAggregate(buckets[key], aggregateRaw(row, key.start))
					pairs = append(pairs, []any{row.ResourceID, row.CollectedAt})
				}

				updates := make(map[string]any)
				for _, column := range resourceSumColumns {
					updates[column] = gorm.Expr(table.bucket + "." + column + " + excluded." + column)
				}

				for _, column := range resourceMaxColumns {
					updates[column] = gorm.Expr("CASE WHEN excluded." + column + " > " + table.bucket + "." + column + " THEN excluded." + column + " ELSE " + table.bucket + "." + column + " END")
				}

				for key, bucket := range buckets {
					row := map[string]any{table.key: key.id, "bucket_start": key.start, "sample_count": bucket.SampleCount, "cpu_count": bucket.CPUCount, "cpu_sum": bucket.CPUSum, "cpu_peak": bucket.CPUPeak, "memory_count": bucket.MemoryCount, "memory_sum": bucket.MemorySum, "memory_peak": bucket.MemoryPeak, "disk_count": bucket.DiskCount, "disk_sum": bucket.DiskSum, "disk_peak": bucket.DiskPeak, "cpu_limit_sum": bucket.CPULimitSum, "memory_limit_sum": bucket.MemoryLimitSum, "disk_limit_sum": bucket.DiskLimitSum, "cpu_unlimited": bucket.CPUUnlimited, "memory_unlimited": bucket.MemoryUnlimited, "disk_unlimited": bucket.DiskUnlimited, "latest_at": bucket.LatestAt}
					if err := tx.Table(table.bucket).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: table.key}, {Name: "bucket_start"}}, DoUpdates: clause.Assignments(updates)}).Create(&row).Error; err != nil {
						return err
					}
				}

				if err := tx.Exec("DELETE FROM "+table.raw+" WHERE ("+table.key+", collected_at) IN ?", pairs).Error; err != nil {
					return err
				}

				processed = len(rows)

				return nil
			})
			if err != nil {
				return err
			}

			if processed == 0 {
				break
			}
		}
	}

	return nil
}

func aggregateResourcePoint(value database.ResourceAggregate, id string) model.ResourceSample {
	point := model.ResourceSample{CollectedAt: value.BucketStart, ReceivedAt: value.LatestAt, SampleCount: value.SampleCount, CPUSamples: value.CPUCount, MemorySamples: value.MemoryCount, DiskSamples: value.DiskCount, Usage: model.ResourceUsage{ServerID: id, State: "sampled"}}
	if value.SampleCount > 0 {
		if value.CPUUnlimited == 0 {
			point.CPULimit = value.CPULimitSum / float64(value.SampleCount)
		}

		if value.MemoryUnlimited == 0 {
			point.MemoryMB = int64(value.MemoryLimitSum / float64(value.SampleCount))
		}

		if value.DiskUnlimited == 0 {
			point.DiskMB = int64(value.DiskLimitSum / float64(value.SampleCount))
		}
	}

	if value.CPUCount > 0 {
		point.Usage.CPUPercent = new(value.CPUSum / float64(value.CPUCount))
		point.CPUPeak = new(value.CPUPeak)
	}

	if value.MemoryCount > 0 {
		point.Usage.MemoryBytes = new(int64(value.MemorySum / float64(value.MemoryCount)))
		point.MemoryPeak = new(value.MemoryPeak)
	}

	if value.DiskCount > 0 {
		point.Usage.DiskBytes = new(int64(value.DiskSum / float64(value.DiskCount)))
		point.DiskPeak = new(value.DiskPeak)
	}

	if value.CPUCount > 0 || value.MemoryCount > 0 {
		point.Usage.SampledAt = value.LatestAt
	} else {
		point.Usage.State = "unknown"
		point.Usage.ContainerError = "Resource samples unavailable in this bucket."
	}

	if value.DiskCount > 0 {
		point.Usage.DiskSampledAt = value.LatestAt
	} else {
		point.Usage.DiskError = "Game-data samples unavailable in this bucket."
	}

	return point
}

// bucketHistory groups pre-aggregated sums in SQL, reading only the small raw tail
// in Go. The transaction prevents double counting or gaps during compaction.
func (r *ResourceHistoryRepository) bucketHistory(ctx context.Context, spark bool, id string, from, to, interval int64, limit int) ([]model.ResourceSample, error) {
	table := resourceTables[0]
	if spark {
		table = resourceTables[1]
	}

	points := []model.ResourceSample{}
	err := database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		slot := fmt.Sprintf("CAST((bucket_start - %d) / %d AS BIGINT)", from, interval)
		selection := make([]string, 0, 1+len(resourceSumColumns)+len(resourceMaxColumns))
		selection = append(selection, fmt.Sprintf("%s * %d + %d AS bucket_start", slot, interval, from))

		for _, column := range resourceSumColumns {
			selection = append(selection, "SUM("+column+") AS "+column)
		}

		for _, column := range resourceMaxColumns {
			selection = append(selection, "MAX("+column+") AS "+column)
		}

		var rolled []database.ResourceAggregate
		if err := tx.Table(table.bucket).Select(selection).Where(table.key+" = ? AND bucket_start >= ? AND bucket_start <= ? AND latest_at <= ?", id, from, to, to).Group(slot).Order("bucket_start").Limit(limit).Scan(&rolled).Error; err != nil {
			return err
		}

		buckets := make(map[int64]*database.ResourceAggregate, len(rolled))
		for _, row := range rolled {
			row := row
			buckets[row.BucketStart] = &row
		}

		var raw []resourceRawRow
		if err := resourceRawQuery(tx, table).Where(table.key+" = ? AND collected_at >= ? AND collected_at <= ?", id, from, to).Order("collected_at").Limit(2049).Find(&raw).Error; err != nil {
			return err
		}

		if len(raw) > 2048 {
			return ErrResourceHistoryPending
		}

		for _, row := range raw {
			start := from + (row.CollectedAt-from)/interval*interval
			if buckets[start] == nil {
				buckets[start] = &database.ResourceAggregate{BucketStart: start}
			}

			mergeResourceAggregate(buckets[start], aggregateRaw(row, start))
		}

		starts := make([]int64, 0, len(buckets))
		for start := range buckets {
			starts = append(starts, start)
		}

		sort.Slice(starts, func(i, j int) bool { return starts[i] < starts[j] })

		usageID := id
		if spark {
			usageID = ""
		}

		for _, start := range starts[:min(limit, len(starts))] {
			points = append(points, aggregateResourcePoint(*buckets[start], usageID))
		}

		return nil
	})

	return points, err
}
