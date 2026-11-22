//go:build !production

package devmock

import (
	"context"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const availabilitySeedMarkerID = 17

// Seed once, including existing development databases. Never advance fabricated
// history on restart, overwrite known observations, or resurrect deleted pages.
func seedAvailabilityFixtures(ctx context.Context, db *gorm.DB) error {
	return database.Transaction(db.WithContext(ctx), func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&seedMarker{}).Where("id = ?", availabilitySeedMarkerID).Count(&count).Error; err != nil || count > 0 {
			return err
		}
		window := model.EmptyAvailabilityHistory(time.Now().Unix())
		page := database.StatusPage{ID: "mock-availability", Slug: "mock-status", Title: "Mock community status"}
		for _, id := range []string{"survival", "creative", "modded", "unavailable"} {
			var servers []database.Server
			if err := tx.Where("id = ?", id).Find(&servers).Error; err != nil {
				return err
			}
			if len(servers) == 0 {
				continue
			}
			server := servers[0]
			page.Entries = append(page.Entries, database.StatusPageEntry{ID: "mock-status-" + id, PageID: page.ID, ServerID: id, Name: server.Name, Position: len(page.Entries)})
			rows := make([]database.ServerResourceBucket, 0, len(window.Points))
			for i := range window.Points {
				online, offline := mockAvailabilityCoverage(id, i)
				start := window.From + int64(i)*window.Interval
				rows = append(rows, database.ServerResourceBucket{ServerID: id, ResourceAggregate: database.ResourceAggregate{BucketStart: start, OnlineSeconds: online, OfflineSeconds: offline, LatestAt: start + window.Interval - 1}})
			}
			conflict := clause.OnConflict{
				Columns:   []clause.Column{{Name: "server_id"}, {Name: "bucket_start"}},
				DoUpdates: clause.AssignmentColumns([]string{"online_seconds", "offline_seconds"}),
				Where:     clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "server_resource_buckets.online_seconds = 0 AND server_resource_buckets.offline_seconds = 0"}}},
			}
			if err := tx.Omit("Server").Clauses(conflict).CreateInBatches(rows, 100).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&database.StatusPage{}).Where("id = ? OR slug = ?", page.ID, page.Slug).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 && len(page.Entries) > 0 {
			if err := tx.Omit("Entries.Server").Create(&page).Error; err != nil {
				return err
			}
		}
		return tx.Create(&seedMarker{ID: availabilitySeedMarkerID}).Error
	})
}

func mockAvailabilityCoverage(id string, index int) (online, offline int64) {
	switch id {
	case "survival":
		switch {
		case index >= 80 && index < 86:
			return 0, 300
		case index >= 180 && index < 184:
			return 0, 0
		case index == 86:
			return 180, 120
		}
	case "creative":
		if index%72 >= 60 {
			return 0, 300
		}
		if index%72 == 59 {
			return 150, 150
		}
	case "modded":
		if index >= 144 {
			return 0, 300
		}
	case "unavailable":
		if index >= 216 {
			return 0, 0
		}
	}
	return 300, 0
}
