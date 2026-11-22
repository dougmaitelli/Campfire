package repository

import (
	"context"
	"errors"
	"sort"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const maxAvailabilityTargets = 50

// AvailabilitySpan is elapsed coverage inferred from a fresh lifecycle observation.
// Unknown time is represented by absence, never by extending an expired status.
type AvailabilitySpan struct {
	ServerID, SparkID string
	From, To          int64
	Online            bool
}

// RecordAvailability splits coverage across bucket boundaries without dropping
// seconds. Lifecycle collection owns the non-overlapping spans; metric rollup
// and this writer update disjoint columns of the same retained buckets.
func (r *ResourceHistoryRepository) RecordAvailability(ctx context.Context, spans []AvailabilitySpan) error {
	if len(spans) == 0 {
		return nil
	}
	ids := make([]string, 0, len(spans))
	for _, span := range spans {
		if span.From < 0 || span.To <= span.From || span.To-span.From > ResourceBucketSeconds {
			return errors.New("invalid availability span")
		}
		ids = append(ids, span.ServerID)
	}
	return database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		placements := make(map[string]string, len(ids))
		for start := 0; start < len(ids); start += 500 {
			var servers []struct{ ID, SparkID string }
			if err := tx.Model(&database.Server{}).Select("id, spark_id").Where("id IN ?", ids[start:min(start+500, len(ids))]).Scan(&servers).Error; err != nil {
				return err
			}
			for _, server := range servers {
				placements[server.ID] = server.SparkID
			}
		}
		type key struct {
			id    string
			start int64
		}
		buckets := make(map[key]*database.ServerResourceBucket)
		for _, span := range spans {
			placement, exists := placements[span.ServerID]
			if !exists || placement != span.SparkID {
				continue
			}
			for at := span.From; at < span.To; {
				start := at / ResourceBucketSeconds * ResourceBucketSeconds
				end := min(span.To, start+ResourceBucketSeconds)
				k := key{span.ServerID, start}
				if buckets[k] == nil {
					buckets[k] = &database.ServerResourceBucket{ServerID: span.ServerID, ResourceAggregate: database.ResourceAggregate{BucketStart: start}}
				}
				if span.Online {
					buckets[k].OnlineSeconds += end - at
				} else {
					buckets[k].OfflineSeconds += end - at
				}
				at = end
			}
		}
		rows := make([]database.ServerResourceBucket, 0, len(buckets))
		for _, bucket := range buckets {
			rows = append(rows, *bucket)
		}
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].ServerID == rows[j].ServerID {
				return rows[i].BucketStart < rows[j].BucketStart
			}
			return rows[i].ServerID < rows[j].ServerID
		})
		if len(rows) == 0 {
			return nil
		}
		return tx.Omit("Server").Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "server_id"}, {Name: "bucket_start"}},
			DoUpdates: clause.Assignments(map[string]any{
				"online_seconds":  gorm.Expr("server_resource_buckets.online_seconds + excluded.online_seconds"),
				"offline_seconds": gorm.Expr("server_resource_buckets.offline_seconds + excluded.offline_seconds"),
			}),
		}).CreateInBatches(&rows, 100).Error
	})
}

// Availability reads bounded coverage for publication-selected servers. Because
// lifecycle collection writes directly to retained buckets, no raw backlog scan
// or write-lock transaction is needed by anonymous page reads.
func (r *ResourceHistoryRepository) Availability(ctx context.Context, ids []string, now int64) (map[string]model.AvailabilityHistory, error) {
	if len(ids) > maxAvailabilityTargets {
		return nil, errors.New("too many availability targets")
	}
	result := make(map[string]model.AvailabilityHistory, len(ids))
	for _, id := range ids {
		result[id] = model.EmptyAvailabilityHistory(now)
	}
	if len(ids) == 0 {
		return result, nil
	}
	window := model.EmptyAvailabilityHistory(now)
	var rows []struct {
		ServerID                                   string
		BucketStart, OnlineSeconds, OfflineSeconds int64
	}
	if err := r.db.WithContext(ctx).Table("server_resource_buckets").Select("server_id, bucket_start, online_seconds, offline_seconds").Where("server_id IN ? AND bucket_start >= ? AND bucket_start < ?", ids, window.From, window.To).Order("server_id, bucket_start").Limit(maxAvailabilityTargets * model.AvailabilityHistoryPoints).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		history := result[row.ServerID]
		point := &history.Points[(row.BucketStart-window.From)/window.Interval]
		// Clamp exceptional overlapping data, preserving known downtime first.
		point.OfflineSeconds = max(int64(0), min(window.Interval, row.OfflineSeconds))
		point.OnlineSeconds = max(int64(0), min(window.Interval-point.OfflineSeconds, row.OnlineSeconds))
	}
	return result, nil
}
