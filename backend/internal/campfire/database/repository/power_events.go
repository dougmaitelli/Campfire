package repository

import (
	"errors"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/events"
	"campfire/internal/shared/limits"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

// observePowerEvents uses persisted samples so panel restarts, replayed
// collections, stale data, and reconnects do not manufacture state changes.
// Producers report facts independently of which notification sinks are enabled.
func observePowerEvents(tx *gorm.DB, publisher *EventPublisher, samples []database.ServerResourceSample) error {
	if len(samples) == 0 {
		return nil
	}

	ids := make([]string, 0, len(samples))
	for _, sample := range samples {
		ids = append(ids, sample.ServerID)
	}

	previous, err := NewResourceHistoryRepository(tx).Latest(tx.Statement.Context, ids)
	if err != nil {
		return err
	}

	for _, sample := range samples {
		old, exists := previous[sample.ServerID]
		if !exists || old.SparkID != sample.SparkID || old.CollectedAt >= sample.CollectedAt || !freshPowerSample(old) || !freshPowerSample(sample) || !stablePowerState(sample.Usage.State) || old.Usage.State == "" || old.Usage.State == model.RuntimeStateUnknown || old.Usage.State == model.RuntimeStateOffline {
			continue
		}

		restarted := sample.Usage.State == model.RuntimeStateRunning && old.Usage.StartedAt > 0 && sample.Usage.StartedAt > old.Usage.StartedAt
		if old.Usage.State == sample.Usage.State && !restarted {
			continue
		}

		var server database.Server

		err := tx.Select("id").Where("id = ? AND spark_id = ?", sample.ServerID, sample.SparkID).Take(&server).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}

		if err != nil {
			return err
		}

		if _, err := publisher.inTransaction(tx).Publish(tx.Statement.Context, events.PowerStateChanged{ServerID: sample.ServerID, Previous: old.Usage.State, Current: sample.Usage.State, Restarted: restarted && old.Usage.State == sample.Usage.State}, time.Unix(sample.CollectedAt, 0)); err != nil {
			return err
		}
	}

	return nil
}

func stablePowerState(state model.RuntimeState) bool {
	return state == model.RuntimeStateRunning || state == model.RuntimeStateStopped || state == model.RuntimeStateExited || state == model.RuntimeStateDead || state == model.RuntimeStatePaused
}

func freshPowerSample(sample database.ServerResourceSample) bool {
	usage := sample.Usage
	return usage.ContainerError == "" && usage.SampledAt > 0 && usage.SampledAt <= sample.ReceivedAt && sample.ReceivedAt-usage.SampledAt <= limits.MinimumFreshSeconds && usage.SampleAgeSeconds >= 0 && usage.SampleAgeSeconds <= limits.MinimumFreshSeconds
}
