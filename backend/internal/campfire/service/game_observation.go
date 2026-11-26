package service

import (
	"context"
	"time"

	"campfire/internal/shared/limits"
	"campfire/internal/shared/model"
)

type gameStateObservation struct {
	View *GameStateView
	// UsageValidUntil is the underlying runtime sample's validity boundary,
	// independent of any consumer's game-state display/publication policy.
	UsageValidUntil int64
}

// cachedGameStates batches stored usage reads and applies the same placement,
// connectivity and freshness rules as monitoring. Callers also gate publication
// on current runtime availability and whether game integration is enabled.
// No Spark or integration requests are issued by this read path.
func (s *DeploymentService) cachedGameStates(ctx context.Context, targets []model.ResourceTarget) ([]gameStateObservation, error) {
	result := make([]gameStateObservation, len(targets))
	if len(targets) == 0 || s.History == nil || s.Games == nil {
		return result, nil
	}

	ids := make([]string, 0, len(targets))
	for _, target := range targets {
		ids = append(ids, target.ID)
	}

	rows, err := s.History.Store.Latest(ctx, ids)
	if err != nil {
		return nil, err
	}

	now := time.Now().Unix()
	interval := int64(s.History.Interval.Seconds())

	for index, target := range targets {
		row, ok := rows[target.ID]
		if !ok || row.SparkID != target.SparkID {
			continue
		}

		usage := ageResourceUsage(row.Usage, row.ReceivedAt, now)

		view := s.monitoredGame(target.ID, target.SparkID, usage, interval, now)
		if view == nil {
			continue
		}

		result[index] = gameStateObservation{
			View:            view,
			UsageValidUntil: now + limits.SampleFreshSeconds(interval) - usage.SampleAgeSeconds,
		}
	}

	return result, nil
}
