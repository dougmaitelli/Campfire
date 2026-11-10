package repository

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/integrations"
	"campfire/internal/shared/limits"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

// RouteEvent atomically matches every enabled provider binding and records one
// immutable outgoing message per matching binding. It performs no network I/O.
func (r *IntegrationRepository) RouteEvent(ctx context.Context, event integrations.Event, now time.Time) error {
	if err := event.Validate(); err != nil {
		return err
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		var bindings []database.ServerIntegration
		if err := tx.Joins("JOIN integrations ON integrations.id = server_integrations.integration_id").
			Where("server_integrations.server_id = ? AND server_integrations.enabled = ? AND integrations.enabled = ?", event.ServerID, true, true).
			Find(&bindings).Error; err != nil {
			return err
		}

		store := NewIntegrationRepository(tx)

		for _, binding := range bindings {
			for _, subscription := range binding.Subscriptions {
				if subscription.Matches(event) {
					if _, err := store.EnqueueMessage(ctx, binding.ID, event.ID, event.Category, payload, now); err != nil {
						return err
					}

					break
				}
			}
		}

		return nil
	})
}

// observeIntegrationPower uses persisted samples so panel restarts, replayed
// collections, stale data, and reconnects do not manufacture state changes.
func observeIntegrationPower(tx *gorm.DB, samples []database.ServerResourceSample) error {
	if len(samples) == 0 {
		return nil
	}

	ids := make([]string, 0, len(samples))
	for _, sample := range samples {
		ids = append(ids, sample.ServerID)
	}

	var bindings []database.ServerIntegration
	if err := tx.Select("server_integrations.server_id", "server_integrations.subscriptions").
		Joins("JOIN integrations ON integrations.id = server_integrations.integration_id").
		Where("server_integrations.server_id IN ? AND server_integrations.enabled = ? AND integrations.enabled = ?", ids, true, true).Find(&bindings).Error; err != nil {
		return err
	}

	interested := map[string]bool{}

	for _, binding := range bindings {
		if slices.ContainsFunc(binding.Subscriptions, func(subscription integrations.Subscription) bool {
			return subscription.Category == integrations.PowerEvents
		}) {
			interested[binding.ServerID] = true
		}
	}

	ids = slices.DeleteFunc(ids, func(id string) bool { return !interested[id] })
	if len(ids) == 0 {
		return nil
	}

	previous, err := NewResourceHistoryRepository(tx).Latest(tx.Statement.Context, ids)
	if err != nil {
		return err
	}

	for _, sample := range samples {
		old, exists := previous[sample.ServerID]
		if !exists || !interested[sample.ServerID] || old.SparkID != sample.SparkID || old.CollectedAt >= sample.CollectedAt || !freshPowerSample(old) || !freshPowerSample(sample) || !stablePowerState(sample.Usage.State) || old.Usage.State == "" || old.Usage.State == model.RuntimeStateUnknown || old.Usage.State == model.RuntimeStateOffline {
			continue
		}

		kind := ""
		if old.Usage.State != sample.Usage.State {
			kind = "server." + string(sample.Usage.State)
		} else if sample.Usage.State == model.RuntimeStateRunning && old.Usage.StartedAt > 0 && sample.Usage.StartedAt > old.Usage.StartedAt {
			kind = "server.restarted"
		}

		if kind == "" {
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

		event := integrations.Event{ID: "power:" + strconv.FormatInt(sample.CollectedAt, 10), ServerID: sample.ServerID, Category: integrations.PowerEvents, Type: kind, Title: "Server state: " + string(sample.Usage.State), OccurredAt: sample.CollectedAt, Data: map[string]any{"state": string(sample.Usage.State), "previous_state": string(old.Usage.State)}}
		if err := NewIntegrationRepository(tx).RouteEvent(tx.Statement.Context, event, time.Unix(sample.CollectedAt, 0)); err != nil {
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
