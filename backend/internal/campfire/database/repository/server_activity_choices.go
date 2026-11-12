package repository

import (
	"context"

	"campfire/internal/campfire/database"
)

type activityGameRevision struct {
	Count        int64
	LastID       int64
	Registration int64
}

type activityGameChoices struct {
	revision activityGameRevision
	options  GameEventFilterOptions
}

// Events and their data are immutable. An indexed count/last-ID query detects
// ingestion and retention without decoding JSON. Registration revisions also
// invalidate cached labels even when no new event has arrived.
func (r *ServerActivityRepository) cachedGameChoices(ctx context.Context, serverID string) (GameEventFilterOptions, error) {
	var revision activityGameRevision
	if err := r.db.WithContext(ctx).Model(&database.GameEvent{}).Select("COUNT(*) AS count, COALESCE(MAX(id), 0) AS last_id").Where("server_id = ?", serverID).Scan(&revision).Error; err != nil {
		return GameEventFilterOptions{}, err
	}

	if err := r.db.WithContext(ctx).Model(&database.GameIntegrationRegistration{}).Select("revision").Where("server_id = ?", serverID).Scan(&revision.Registration).Error; err != nil {
		return GameEventFilterOptions{}, err
	}

	r.choicesMu.Lock()
	cached, exists := r.gameChoices[serverID]
	r.choicesMu.Unlock()

	if exists && cached.revision == revision {
		return cached.options, nil
	}

	options, err := NewGameEventRepository(r.db).FilterOptions(ctx, serverID)
	if err != nil {
		return options, err
	}

	r.choicesMu.Lock()
	defer r.choicesMu.Unlock()
	// Bound memory independently of the number of servers visited over time.
	if len(r.gameChoices) >= 8 {
		clear(r.gameChoices)
	}

	r.gameChoices[serverID] = activityGameChoices{revision: revision, options: options}

	return options, nil
}
