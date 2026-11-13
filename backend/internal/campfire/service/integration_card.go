package service

import (
	"context"
	"time"

	core "campfire/internal/campfire/integrations"
	"campfire/internal/shared/limits"
	"campfire/internal/shared/model"
)

// IntegrationCard reads saved public metadata and already-collected runtime
// data. Building or previewing a card never contacts Spark or Discord.
func (s *DeploymentService) IntegrationCard(ctx context.Context, id string, settings core.CardSettings) (core.ServerCard, error) {
	if err := settings.Validate(); err != nil {
		return core.ServerCard{}, err
	}

	server, err := s.Servers.Get(ctx, id)
	if err != nil {
		return core.ServerCard{}, err
	}

	state := model.RuntimeStateUnknown
	expires := int64(0)

	s.mu.Lock()
	if status, ok := s.statuses[id]; ok && s.statusSparks[id] == server.SparkID && time.Since(s.observed[id]) < runtimeStatusLifetime {
		state = status.State
		expires = s.observed[id].Add(runtimeStatusLifetime).Unix()
	}
	s.mu.Unlock()

	var game *model.GameState

	if s.History != nil && server.GameIntegration && state == model.RuntimeStateRunning {
		rows, err := s.History.Store.Latest(ctx, []string{id})
		if err != nil {
			return core.ServerCard{}, err
		}

		if row, ok := rows[id]; ok && row.SparkID == server.SparkID {
			now := time.Now().Unix()

			usage := ageResourceUsage(row.Usage, row.ReceivedAt, now)
			if view := s.monitoredGame(id, server.SparkID, usage, int64(s.History.Interval.Seconds()), now); view != nil {
				game = view.State
				if game != nil {
					expires = min(expires, view.UpdatedAt+view.FreshForSeconds, now+limits.SampleFreshSeconds(int64(s.History.Interval.Seconds()))-usage.SampleAgeSeconds)
				}
			}
		}
	}

	var labels model.GameFieldLabels
	if s.Games != nil {
		labels, err = s.Games.StateLabels(ctx, id)
		if err != nil {
			return core.ServerCard{}, err
		}
	}

	card := core.BuildServerCard(server, state, game, settings, labels)
	card.ExpiresAt = expires

	return card, nil
}
