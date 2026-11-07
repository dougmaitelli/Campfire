package service

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"campfire/internal/shared/model"
	accessperm "campfire/internal/shared/permissions"
)

const runtimeStatusInterval = 3 * time.Second
const runtimeStatusLifetime = 15 * time.Second

// RuntimeStatusService collects once per fleet, independently of browser count.
type RuntimeStatusService struct {
	deployment *DeploymentService
	collectMu  sync.Mutex
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

// NewRuntimeStatusService constructs the shared lifecycle status collector.
func NewRuntimeStatusService(deployment *DeploymentService) *RuntimeStatusService {
	return &RuntimeStatusService{deployment: deployment}
}

// Start begins bounded background collection, including an immediate sample.
func (s *RuntimeStatusService) Start(ctx context.Context) {
	ctx, s.cancel = context.WithCancel(ctx)

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		ticker := time.NewTicker(runtimeStatusInterval)
		defer ticker.Stop()

		for {
			if err := s.Collect(ctx); err != nil && ctx.Err() == nil {
				slog.Error("collect runtime status", "error", err)
			}

			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

// Close cancels collection and waits for active inspections to exit.
func (s *RuntimeStatusService) Close() {
	if s.cancel != nil {
		s.cancel()
	}

	s.wg.Wait()
}

// Collect refreshes runtime status with one bounded worker pool for the fleet.
func (s *RuntimeStatusService) Collect(ctx context.Context) error {
	s.collectMu.Lock()
	defer s.collectMu.Unlock()

	d := s.deployment

	targets, err := d.History.Store.Targets(ctx, 0, true)
	if err != nil {
		return err
	}

	nodes, err := d.Sparks.List(ctx)
	if err != nil {
		return err
	}

	online := make(map[string]bool, len(nodes))
	for _, node := range nodes {
		online[node.ID] = node.Connected && !node.Disabled
	}

	present := make(map[string]bool, len(targets))
	for _, target := range targets {
		present[target.ID] = true
	}

	d.mu.Lock()
	for id := range d.statuses {
		if !present[id] {
			delete(d.statuses, id)
			delete(d.observed, id)
			delete(d.statusSparks, id)
			delete(d.delivered, id)
		}
	}
	d.mu.Unlock()

	jobs := make(chan model.ResourceTarget)

	var workers sync.WaitGroup
	for range min(resourceSamplingConcurrency, len(targets)) {
		workers.Add(1)
		go func() {
			defer workers.Done()

			for target := range jobs {
				if !online[target.SparkID] {
					d.cacheStatus(target.SparkID, model.RuntimeStatus{ServerID: target.ID, State: model.RuntimeStateUnknown})
					continue
				}

				sample, cancel := context.WithTimeout(ctx, resourceSampleTimeout)
				if err := d.syncChanged(sample, target.ID); err != nil && sample.Err() == nil {
					slog.Warn("deliver saved server configuration", "server", target.ID, "error", err)
				}

				_, err := d.Action(sample, target.ID, model.ServerActionStatus)

				cancel()

				if err != nil && ctx.Err() == nil {
					d.cacheStatus(target.SparkID, model.RuntimeStatus{ServerID: target.ID, State: model.RuntimeStateUnknown})
				}
			}
		}()
	}

	for _, target := range targets {
		select {
		case jobs <- target:
		case <-ctx.Done():
			close(jobs)
			workers.Wait()

			return ctx.Err()
		}
	}

	close(jobs)
	workers.Wait()

	return nil
}

func (s *DeploymentService) cacheStatus(sparkID string, status model.RuntimeStatus) {
	// Server viewing does not grant console access, and metrics have their own API.
	status.Logs, status.Metrics, status.LogObservation = "", nil, nil

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.statuses == nil {
		s.statuses = make(map[string]model.RuntimeStatus)
		s.observed = make(map[string]time.Time)
	}

	if s.statusSparks == nil {
		s.statusSparks = make(map[string]string)
	}

	s.statuses[status.ServerID] = status
	s.observed[status.ServerID] = time.Now()
	s.statusSparks[status.ServerID] = sparkID
}

// RuntimeStatuses authorizes every read; it never contacts Spark or Docker.
func (s *DeploymentService) RuntimeStatuses(ctx context.Context, principal Principal) ([]model.RuntimeStatus, error) {
	result := []model.RuntimeStatus{}
	if !principal.tokenAllows(accessperm.ServerView) {
		return result, nil
	}

	if s.History == nil {
		return nil, ErrDeploymentUnavailable
	}

	targets, err := s.History.Store.Targets(ctx, principal.UserID, principal.Admin)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, target := range targets {
		status, ok := s.statuses[target.ID]
		if !ok || s.statusSparks[target.ID] != target.SparkID || time.Since(s.observed[target.ID]) >= runtimeStatusLifetime {
			status = model.RuntimeStatus{ServerID: target.ID, State: model.RuntimeStateUnknown}
		}

		result = append(result, status)
	}

	return result, nil
}
