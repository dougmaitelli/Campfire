//go:build !production

package devmock

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

const crashSeedMarkerID = 16

// seedCrashFixtures adds historical evidence without changing simulated power
// state or operations. The marker also upgrades existing development databases
// once, without refreshing retention or resurrecting removed examples on restart.
func seedCrashFixtures(ctx context.Context, db *gorm.DB) error {
	return database.Transaction(db.WithContext(ctx), func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&seedMarker{}).Where("id = ?", crashSeedMarkerID).Count(&count).Error; err != nil || count > 0 {
			return err
		}

		var server database.Server
		if err := tx.Take(&server, "id = ?", "survival").Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Create(&seedMarker{ID: crashSeedMarkerID}).Error
		} else if err != nil {
			return err
		}

		now := time.Now().UTC()

		records := []model.CrashRecord{
			{ExitCode: 137, Reason: model.CrashReasonOutOfMemory, Console: "[Server thread/INFO] Saving world before shutdown\n[Server thread/ERROR] Native memory allocation failed (mock crash evidence).\n", ConsoleTruncated: true},
			{ExitCode: 137, Reason: model.CrashReasonUnknown, Console: "[Server thread/INFO] Loading world\n[Server thread/ERROR] Process terminated; Docker OOM evidence was unavailable (mock crash evidence).\n"},
			{ExitCode: 1, Reason: model.CrashReasonUnknown, Console: "[Server thread/ERROR] Unhandled game exception (mock crash evidence).\njava.lang.IllegalStateException: Example plugin startup failure\n"},
			{ExitCode: 2, Reason: model.CrashReasonUnknown, ConsoleUnavailable: true},
		}
		for index := range records {
			record := &records[index]
			record.ID = fmt.Sprintf("%x", sha256.Sum256(fmt.Appendf(nil, "mock:survival:crash:%d", index)))
			record.ContainerID = fmt.Sprintf("%x", sha256.Sum256([]byte("mock:survival:crash-container")))
			record.ExitedAt = now.Add(-time.Duration(index+1) * time.Hour).Unix()
		}

		if err := repository.NewCrashRepository(tx).Observe(ctx, server.SparkID, server.ID, records); err != nil {
			return err
		}

		return tx.Create(&seedMarker{ID: crashSeedMarkerID}).Error
	})
}
