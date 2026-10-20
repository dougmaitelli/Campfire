package service

import (
	"context"
	"errors"

	"campfire/internal/shared/model"
)

// ErrSparkPrune describes an unavailable node cleanup request.
var ErrSparkPrune = errors.New("spark must be connected and enabled before pruning")

// Prune reclaims only unused Campfire Docker resources on a connected Spark.
func (s *SparkService) Prune(ctx context.Context, id string) (model.PruneResult, error) {
	spark, err := s.Get(ctx, id)
	if err != nil {
		return model.PruneResult{}, err
	}

	pruner, ok := s.connectivity.(interface {
		PruneSpark(context.Context, string) (model.PruneResult, error)
	})
	if !ok || !spark.Connected || spark.Disabled {
		return model.PruneResult{}, ErrSparkPrune
	}

	result, err := pruner.PruneSpark(ctx, id)
	if err != nil {
		return result, errors.Join(ErrDeploymentUnavailable, err)
	}

	return result, nil
}
