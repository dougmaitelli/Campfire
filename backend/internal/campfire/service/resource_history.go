package service

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

// Default collection and retention keep resource history bounded independently of readers.
const (
	DefaultResourceInterval     = 10 * time.Second
	DefaultResourceRetention    = 30 * 24 * time.Hour
	resourceCleanupInterval     = time.Hour
	resourceRollupInterval      = 2 * time.Minute
	resourceSamplingConcurrency = 6
	resourceSampleTimeout       = 5 * time.Second
	MaxResourceHistoryPoints    = 1000
)

// ErrResourceHistoryInput rejects unbounded or malformed graph queries.
var ErrResourceHistoryInput = errors.New("invalid resource history range or point limit")

// ResourceHistoryService collects once for the fleet, independent of page reads.
type ResourceHistoryService struct {
	Store      *repository.ResourceHistoryRepository
	Interval   time.Duration
	Retention  time.Duration
	deployment *DeploymentService
	collectMu  sync.Mutex
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

// NewResourceHistoryService constructs fleet sampling, compaction and retention jobs.
func NewResourceHistoryService(store *repository.ResourceHistoryRepository, deployment *DeploymentService) *ResourceHistoryService {
	return &ResourceHistoryService{Store: store, deployment: deployment, Interval: DefaultResourceInterval, Retention: DefaultResourceRetention}
}

// Start begins bounded background jobs, including immediate collection and maintenance.
func (s *ResourceHistoryService) Start(ctx context.Context) error {
	if s.Interval < time.Second || s.Interval > 5*time.Minute || s.Retention < 24*time.Hour || s.Retention > 365*24*time.Hour {
		return errors.New("resource sampling interval must be 1–300 seconds and retention 1–365 days")
	}

	ctx, s.cancel = context.WithCancel(ctx)

	s.wg.Add(3)
	go func() {
		defer s.wg.Done()

		ticker := time.NewTicker(s.Interval)
		defer ticker.Stop()

		for {
			if err := s.Collect(ctx); err != nil && ctx.Err() == nil {
				slog.Error("collect resource history", "error", err)
			}

			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	go func() {
		defer s.wg.Done()

		ticker := time.NewTicker(resourceCleanupInterval)
		defer ticker.Stop()

		for {
			cleanup, cancel := context.WithTimeout(ctx, 30*time.Second)
			err := s.Store.Prune(cleanup, time.Now().Add(-s.Retention).Unix())

			cancel()

			if err != nil && ctx.Err() == nil {
				slog.Error("prune resource history", "error", err)
			}

			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	go func() {
		defer s.wg.Done()

		ticker := time.NewTicker(resourceRollupInterval)
		defer ticker.Stop()

		for {
			rollup, cancel := context.WithTimeout(ctx, time.Minute)
			err := s.Store.Rollup(rollup, time.Now().Unix())

			cancel()

			if err != nil && ctx.Err() == nil {
				slog.Error("roll up resource history", "error", err)
			}

			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	return nil
}

// Close cancels sampling and maintenance and waits for all jobs to exit.
func (s *ResourceHistoryService) Close() {
	if s.cancel != nil {
		s.cancel()
	}

	s.wg.Wait()
}

// Collect records unavailable samples as gaps, rather than carrying old values forward.
func (s *ResourceHistoryService) Collect(ctx context.Context) error {
	if !s.collectMu.TryLock() {
		return errors.New("resource collection is already running")
	}
	defer s.collectMu.Unlock()

	cycle, cancel := context.WithTimeout(ctx, max(30*time.Second, 2*s.Interval))
	defer cancel()

	targets, err := s.Store.Targets(cycle, 0, true)
	if err != nil {
		return err
	}

	nodes, err := s.deployment.Sparks.List(cycle)
	if err != nil {
		return err
	}

	if s.deployment.Games != nil {
		s.deployment.Games.Retain(targets)
	}

	nodeMap := make(map[string]model.Spark, len(nodes))
	for _, node := range nodes {
		nodeMap[node.ID] = node
	}

	collected := time.Now().Unix()

	rows := make([]database.ServerResourceSample, len(targets))
	for i, target := range targets {
		rows[i] = database.ServerResourceSample{ServerID: target.ID, SparkID: target.SparkID, CollectedAt: collected, ReceivedAt: collected, CPULimit: target.CPULimit, MemoryMB: target.MemoryMB, DiskMB: target.DiskMB, Usage: unknownUsage(target.ID)}
	}

	jobs := make(chan int)

	var wg sync.WaitGroup
	for range min(resourceSamplingConcurrency, len(targets)) {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for i := range jobs {
				target := targets[i]

				node := nodeMap[target.SparkID]
				if node.Disabled || !node.Connected {
					continue
				}

				usage, err := s.sample(cycle, target)
				if err == nil {
					rows[i].Usage = usage
				}

				rows[i].ReceivedAt = time.Now().Unix()
			}
		}()
	}

queue:
	for i := range targets {
		select {
		case jobs <- i:
		case <-cycle.Done():
			break queue
		}
	}

	close(jobs)
	wg.Wait()

	byNode := make(map[string][]database.ServerResourceSample, len(nodes))
	for _, row := range rows {
		byNode[row.SparkID] = append(byNode[row.SparkID], row)
	}

	sparkRows := make([]database.SparkResourceSample, 0, len(nodes))
	for _, node := range nodes {
		usage := aggregateResources(node, byNode[node.ID], time.Now().Unix())
		sparkRows = append(sparkRows, database.SparkResourceSample{SparkID: node.ID, CollectedAt: collected, ReceivedAt: time.Now().Unix(), CPULimit: node.CPULimit, MemoryMB: node.MemoryMB, DiskMB: node.DiskMB, Usage: usage})
	}

	saveCtx, saveCancel := context.WithTimeout(ctx, 10*time.Second)
	defer saveCancel()

	return s.Store.Save(saveCtx, rows, sparkRows)
}

func (s *ResourceHistoryService) sample(ctx context.Context, target model.ResourceTarget) (model.ResourceUsage, error) {
	began := time.Now()

	ctx, cancel := context.WithTimeout(ctx, resourceSampleTimeout)
	defer cancel()

	dispatch, ok := s.deployment.Remote.(RuntimeDispatcher)
	if !ok {
		return model.ResourceUsage{}, ErrDeploymentUnavailable
	}

	status, err := dispatch.Action(ctx, target.SparkID, model.RuntimeRequest{ServerID: target.ID, Action: "metrics"})
	if err != nil || status.ServerID != target.ID || status.Metrics == nil || status.Metrics.ServerID != target.ID || !validUsage(*status.Metrics) {
		return model.ResourceUsage{}, ErrDeploymentUnavailable
	}

	if s.deployment.Games != nil {
		s.deployment.Games.Observe(target.ID, target.SparkID, *status.Metrics, began, status.Busy)
	}

	return *status.Metrics, nil
}

func aggregateResources(node model.Spark, rows []database.ServerResourceSample, collected int64) model.ResourceUsage {
	usage := unknownUsage("")
	if node.Disabled || !node.Connected {
		return usage
	}

	usage = model.ResourceUsage{State: "connected", CPUPercent: new(float64), MemoryBytes: new(int64), DiskBytes: new(int64), SampledAt: collected, DiskSampledAt: collected}

	for _, row := range rows {
		if row.SparkID != node.ID {
			continue
		}

		current := row.Usage
		if usage.CPUPercent != nil && current.CPUPercent != nil {
			*usage.CPUPercent += *current.CPUPercent
		} else {
			usage.CPUPercent = nil
		}

		if usage.MemoryBytes != nil && current.MemoryBytes != nil {
			*usage.MemoryBytes += *current.MemoryBytes
		} else {
			usage.MemoryBytes = nil
		}

		if usage.DiskBytes != nil && current.DiskBytes != nil {
			*usage.DiskBytes += *current.DiskBytes
		} else {
			usage.DiskBytes = nil
		}

		usage.SampledAt = min(usage.SampledAt, current.SampledAt)
		usage.DiskSampledAt = min(usage.DiskSampledAt, current.DiskSampledAt)
		usage.SampleAgeSeconds = max(usage.SampleAgeSeconds, current.SampleAgeSeconds+collected-row.ReceivedAt)

		usage.DiskAgeSeconds = max(usage.DiskAgeSeconds, current.DiskAgeSeconds+collected-row.ReceivedAt)
		if current.ContainerError != "" {
			usage.ContainerError = "Some game-server measurements are unavailable."
		}

		if current.DiskError != "" {
			usage.DiskError = "Some game-data measurements are unavailable."
		}
	}

	return usage
}

func ageResourceUsage(usage model.ResourceUsage, received, now int64) model.ResourceUsage {
	delay := max(int64(0), now-received)
	usage.SampleAgeSeconds += delay
	usage.DiskAgeSeconds += delay

	return usage
}

func (s *ResourceHistoryService) latestUsage(ctx context.Context, id string) (model.ResourceUsage, error) {
	rows, err := s.Store.Latest(ctx, []string{id})
	if err != nil {
		return model.ResourceUsage{}, err
	}

	row, found := rows[id]
	if !found {
		return unknownUsage(id), nil
	}

	return ageResourceUsage(row.Usage, row.ReceivedAt, time.Now().Unix()), nil
}

// History bounds display buckets while merging weighted rollups and the raw tail.
func (s *ResourceHistoryService) History(ctx context.Context, spark bool, id string, from, to int64, limit int) (model.ResourceHistory, error) {
	if to == 0 {
		to = time.Now().Unix()
	}

	if from == 0 {
		from = max(int64(0), to-86400)
	}

	if limit == 0 {
		limit = 240
	}

	if from < 0 || to < from || to > time.Now().Add(5*time.Minute).Unix() || to-from > int64((365*24*time.Hour).Seconds()) || limit < 1 || limit > MaxResourceHistoryPoints {
		return model.ResourceHistory{}, ErrResourceHistoryInput
	}

	from = from / repository.ResourceBucketSeconds * repository.ResourceBucketSeconds
	interval := (to - from + int64(limit)) / int64(limit)
	interval = max(repository.ResourceBucketSeconds, (interval+repository.ResourceBucketSeconds-1)/repository.ResourceBucketSeconds*repository.ResourceBucketSeconds)
	points, err := s.Store.History(ctx, spark, id, from, to, interval, limit)

	return model.ResourceHistory{From: from, To: to, IntervalSeconds: interval, Samples: points}, err
}
