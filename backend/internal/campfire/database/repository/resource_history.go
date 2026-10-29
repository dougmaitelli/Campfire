package repository

import (
	"context"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"
	accessperm "campfire/internal/shared/permissions"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ResourceHistoryRepository stores shared samples and filters inventory in SQL.
type ResourceHistoryRepository struct{ db *gorm.DB }

// NewResourceHistoryRepository constructs persistent resource history storage.
func NewResourceHistoryRepository(db *gorm.DB) *ResourceHistoryRepository {
	return &ResourceHistoryRepository{db: db}
}

// Targets avoids loading Ember definitions and per-server permission queries.
func (r *ResourceHistoryRepository) Targets(ctx context.Context, userID int64, admin bool) ([]model.ResourceTarget, error) {
	rows := []model.ResourceTarget{}

	query := r.db.WithContext(ctx).Table("servers").
		Select("servers.id, servers.name, servers.spark_id, sparks.name AS spark_name, servers.cpu_limit, servers.memory_mb, servers.disk_mb").
		Joins("JOIN sparks ON sparks.id = servers.spark_id")
	if !admin {
		query = query.Where(`servers.owner_user_id = ? OR EXISTS (
SELECT 1 FROM server_access_grants grants
JOIN role_permissions permissions ON permissions.role_id = grants.access_role_id AND permissions.permission = ?
JOIN users ON users.id = ? AND users.deleted_at IS NULL AND users.disabled = false
WHERE grants.server_id = servers.id AND (grants.user_id = users.id OR grants.role_id = users.role_id))`, userID, accessperm.ServerView, userID)
	}

	err := query.Order("servers.id").Scan(&rows).Error

	return rows, err
}

// Save commits one collection cycle; resources deleted during sampling are omitted.
func (r *ResourceHistoryRepository) Save(ctx context.Context, servers []database.ServerResourceSample, sparks []database.SparkResourceSample) error {
	return database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		var serverIDs, sparkIDs []string
		if err := tx.Model(&database.Server{}).Pluck("id", &serverIDs).Error; err != nil {
			return err
		}

		if err := tx.Model(&database.Spark{}).Pluck("id", &sparkIDs).Error; err != nil {
			return err
		}

		present := make(map[string]bool, len(serverIDs))
		for _, id := range serverIDs {
			present[id] = true
		}

		keptServers := make([]database.ServerResourceSample, 0, len(servers))
		for _, sample := range servers {
			if present[sample.ServerID] {
				keptServers = append(keptServers, sample)
			}
		}

		present = make(map[string]bool, len(sparkIDs))
		for _, id := range sparkIDs {
			present[id] = true
		}

		keptSparks := make([]database.SparkResourceSample, 0, len(sparks))
		for _, sample := range sparks {
			if present[sample.SparkID] {
				keptSparks = append(keptSparks, sample)
			}
		}

		if len(keptServers) > 0 {
			if err := tx.Omit("Server").Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(keptServers, 50).Error; err != nil {
				return err
			}
		}

		if len(keptSparks) > 0 {
			return tx.Omit("Spark").Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(keptSparks, 50).Error
		}

		return nil
	})
}

// LatestSparks reads the collector's persisted totals without resampling nodes.
func (r *ResourceHistoryRepository) LatestSparks(ctx context.Context, ids []string) (map[string]database.SparkResourceSample, error) {
	result := make(map[string]database.SparkResourceSample, len(ids))
	for start := 0; start < len(ids); start += 500 {
		batch := ids[start:min(start+500, len(ids))]
		latest := r.db.Model(&database.SparkResourceSample{}).Select("spark_id, MAX(collected_at) AS newest").Where("spark_id IN ?", batch).Group("spark_id")

		var rows []database.SparkResourceSample

		err := r.db.WithContext(ctx).Model(&database.SparkResourceSample{}).
			Joins("JOIN (?) latest ON latest.spark_id = spark_resource_samples.spark_id AND latest.newest = spark_resource_samples.collected_at", latest).
			Find(&rows).Error
		if err != nil {
			return nil, err
		}

		for _, row := range rows {
			result[row.SparkID] = row
		}
	}

	return result, nil
}

// Latest fetches all requested latest samples with indexed grouped queries.
func (r *ResourceHistoryRepository) Latest(ctx context.Context, ids []string) (map[string]database.ServerResourceSample, error) {
	result := make(map[string]database.ServerResourceSample, len(ids))
	for start := 0; start < len(ids); start += 500 {
		batch := ids[start:min(start+500, len(ids))]
		latest := r.db.Model(&database.ServerResourceSample{}).Select("server_id, MAX(collected_at) AS newest").Where("server_id IN ?", batch).Group("server_id")

		var rows []database.ServerResourceSample

		err := r.db.WithContext(ctx).Model(&database.ServerResourceSample{}).
			Joins("JOIN (?) latest ON latest.server_id = server_resource_samples.server_id AND latest.newest = server_resource_samples.collected_at", latest).
			Joins("JOIN servers current_server ON current_server.id = server_resource_samples.server_id AND current_server.spark_id = server_resource_samples.spark_id").
			Find(&rows).Error
		if err != nil {
			return nil, err
		}

		for _, row := range rows {
			result[row.ServerID] = row
		}
	}

	return result, nil
}

// History combines persisted rollups and the unrolled raw tail.
func (r *ResourceHistoryRepository) History(ctx context.Context, spark bool, id string, from, to, interval int64, limit int) ([]model.ResourceSample, error) {
	return r.bucketHistory(ctx, spark, id, from, to, interval, limit)
}

// Prune deletes expired rows in short batches on SQLite and PostgreSQL alike.
func (r *ResourceHistoryRepository) Prune(ctx context.Context, before int64) error {
	for _, target := range []struct{ table, key, timestamp string }{{"server_resource_samples", "server_id", "collected_at"}, {"spark_resource_samples", "spark_id", "collected_at"}, {"server_resource_buckets", "server_id", "bucket_start"}, {"spark_resource_buckets", "spark_id", "bucket_start"}} {
		cutoff := before
		if target.timestamp == "bucket_start" {
			cutoff = before / ResourceBucketSeconds * ResourceBucketSeconds
		}

		for {
			var changed int64

			err := database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
				result := tx.Exec("DELETE FROM "+target.table+" WHERE ("+target.key+", "+target.timestamp+") IN (SELECT "+target.key+", "+target.timestamp+" FROM "+target.table+" WHERE "+target.timestamp+" < ? ORDER BY "+target.timestamp+" LIMIT 1000)", cutoff)
				changed = result.RowsAffected

				return result.Error
			})
			if err != nil {
				return err
			}

			if changed == 0 {
				break
			}
		}
	}

	return nil
}
