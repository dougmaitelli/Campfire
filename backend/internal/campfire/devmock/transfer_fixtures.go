//go:build !production

package devmock

import (
	"context"
	"errors"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

const transferHistorySeedMarkerID = 9

func seedTransferHistory(ctx context.Context, db *gorm.DB) error {
	return database.Transaction(db.WithContext(ctx), func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&seedMarker{}).Where("id = ?", transferHistorySeedMarkerID).Count(&count).Error; err != nil || count > 0 {
			return err
		}

		server, err := repository.NewServerRepository(tx).Get(ctx, "creative")
		if errors.Is(err, repository.ErrServerNotFound) {
			return tx.Create(&seedMarker{ID: transferHistorySeedMarkerID}).Error
		} else if err != nil {
			return err
		}

		otherSpark := "mock-america"
		if server.SparkID == otherSpark {
			otherSpark = "mock-europe"
		}

		current, other := server.ServerConfiguration, server.ServerConfiguration
		other.SparkID = otherSpark
		now := time.Now().UTC()

		for _, item := range []database.ServerTransfer{
			{
				ID: "mock-completed-transfer", ServerID: server.ID, SourceSparkID: other.SparkID, TargetSparkID: current.SparkID,
				SourceConfiguration: other, TargetConfiguration: current, Phase: model.TransferPhaseComplete,
				TransferredBytes: 768 << 20, TotalBytes: 768 << 20,
				CreatedAt: now.Add(-48 * time.Hour).Unix(), UpdatedAt: now.Add(-48*time.Hour + 3*time.Minute).Unix(),
			},
			{
				ID: "mock-cancelled-transfer", ServerID: server.ID, SourceSparkID: current.SparkID, TargetSparkID: other.SparkID,
				SourceConfiguration: current, TargetConfiguration: other, Phase: model.TransferPhaseCancelled, CancelFrom: model.TransferPhaseCopying,
				TransferredBytes: 256 << 20, TotalBytes: 768 << 20,
				CreatedAt: now.Add(-24 * time.Hour).Unix(), UpdatedAt: now.Add(-24*time.Hour + time.Minute).Unix(),
			},
		} {
			if err := tx.Create(&item).Error; err != nil {
				return err
			}
		}

		return tx.Create(&seedMarker{ID: transferHistorySeedMarkerID}).Error
	})
}
