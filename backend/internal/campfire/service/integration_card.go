package service

import (
	"context"
	"time"

	core "campfire/internal/campfire/integrations"
	"campfire/internal/shared/model"
)

// External cards must expire publications without relying on a browser clock.
// Browser game-state freshness is independently owned by the frontend.
const integrationCardGameFreshFor = 60 * time.Second

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
		observations, err := s.cachedGameStates(ctx, []model.ResourceTarget{{ID: id, SparkID: server.SparkID}})
		if err != nil {
			return core.ServerCard{}, err
		}

		observation := observations[0]
		if observation.View != nil && observation.View.State != nil {
			gameExpires := min(observation.UsageValidUntil, observation.View.UpdatedAt+int64(integrationCardGameFreshFor.Seconds()))
			if gameExpires > time.Now().Unix() {
				game = observation.View.State
				expires = min(expires, gameExpires)
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
