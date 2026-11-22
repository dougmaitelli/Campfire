package service

import (
	"context"
	"time"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/statuspage"
	"campfire/internal/shared/model"
)

type availabilityObservation struct {
	sparkID     string
	state       statuspage.State
	at, expires int64
}

// recordAvailability runs under the lifecycle collector's collectMu. It closes
// elapsed spans before adopting the next observation, independent of the CPU /
// memory sampling interval. A fresh process never fills its unobserved downtime.
func (s *RuntimeStatusService) recordAvailability(ctx context.Context, targets []model.ResourceTarget, observations []runtimeStatusObservation, now time.Time) error {
	next := make(map[string]availabilityObservation, len(targets))
	spans := make([]repository.AvailabilitySpan, 0, len(targets))
	for i, target := range targets {
		previous, exists := s.availability[target.ID]
		at := now.Unix()
		if exists {
			// Never count the same wall-clock seconds twice after a backward clock step.
			at = max(at, previous.at)
			end := min(now.Unix(), previous.expires)
			if previous.sparkID == target.SparkID && end > previous.at && (previous.state == statuspage.Online || previous.state == statuspage.Offline) {
				spans = append(spans, repository.AvailabilitySpan{ServerID: target.ID, SparkID: target.SparkID, From: previous.at, To: end, Online: previous.state == statuspage.Online})
			}
		}
		current := availabilityObservation{sparkID: target.SparkID, state: statuspage.Unknown, at: at}
		observation := observations[i]
		if !observation.Observed.IsZero() && !observation.Observed.After(now) && now.Sub(observation.Observed) < runtimeStatusLifetime {
			current.state = publicAvailability(observation.Status)
			current.expires = observation.Observed.Add(runtimeStatusLifetime).Unix()
		}
		next[target.ID] = current
	}
	if err := s.deployment.History.Store.RecordAvailability(ctx, spans); err != nil {
		// The transaction rolled back; retaining the previous cursor permits retry
		// without duplicating coverage or extending it beyond its original freshness.
		return err
	}
	s.availability = next
	return nil
}
