package service

import (
	"context"
	"math"
	"time"

	"campfire/internal/shared/model"
)

// MonitoredServer combines authorized metadata and the latest persisted usage.
type MonitoredServer struct {
	ID         string              `json:"id"`
	Name       string              `json:"name"`
	SparkID    string              `json:"spark_id"`
	SparkName  string              `json:"spark_name"`
	CPULimit   float64             `json:"cpu_limit"`
	MemoryMB   int64               `json:"memory_mb"`
	DiskMB     int64               `json:"disk_mb"`
	Usage      model.ResourceUsage `json:"usage"`
	ReceivedAt int64               `json:"received_at"`
	Game       *GameStateView      `json:"game,omitempty"`
}

// MonitoredSpark includes the collector's fleet-wide usage totals for a node.
type MonitoredSpark struct {
	model.Spark
	Usage model.ResourceUsage `json:"usage"`
}

// MonitoringSnapshot omits node inventory for non-administrators.
type MonitoringSnapshot struct {
	CollectedAt           int64             `json:"collected_at"`
	SampleIntervalSeconds int64             `json:"sample_interval_seconds"`
	Servers               []MonitoredServer `json:"servers"`
	Sparks                []MonitoredSpark  `json:"sparks,omitempty"`
	Incomplete            bool              `json:"incomplete"`
}

func unknownUsage(id string) model.ResourceUsage {
	return model.ResourceUsage{ServerID: id, State: "unknown", ContainerError: "Spark, Docker or usage sampling unavailable.", DiskError: "Game-data size unavailable."}
}

func validUsage(usage model.ResourceUsage) bool {
	if usage.StartedAt < 0 || usage.StoppedAt < 0 {
		return false
	}

	if usage.CPUPercent != nil && (math.IsNaN(*usage.CPUPercent) || math.IsInf(*usage.CPUPercent, 0) || *usage.CPUPercent < 0) {
		return false
	}

	if usage.MemoryBytes != nil && *usage.MemoryBytes < 0 || usage.DiskBytes != nil && *usage.DiskBytes < 0 || usage.SampledAt < 0 || usage.DiskSampledAt < 0 || usage.SampleAgeSeconds < 0 || usage.DiskAgeSeconds < 0 {
		return false
	}

	if len(usage.State) > 64 || len(usage.ContainerError) > 256 || len(usage.DiskError) > 256 || len(usage.DiskQuotaError) > 256 {
		return false
	}

	return true
}

// Metrics reads stored measurements; HTTP reads never trigger node sampling.
func (s *DeploymentService) Metrics(ctx context.Context, id string) (model.ResourceUsage, error) {
	if s.History == nil {
		return model.ResourceUsage{}, ErrDeploymentUnavailable
	}

	return s.History.latestUsage(ctx, id)
}

// Monitoring authorizes inventory in one query and loads its latest stored samples.
func (s *DeploymentService) Monitoring(ctx context.Context, principal Principal) (MonitoringSnapshot, error) {
	if s.History == nil {
		return MonitoringSnapshot{}, ErrDeploymentUnavailable
	}

	history := s.History

	snapshot := MonitoringSnapshot{CollectedAt: time.Now().Unix(), SampleIntervalSeconds: int64(history.Interval.Seconds()), Servers: []MonitoredServer{}}
	if principal.tokenAllows("server.view") {
		targets, err := history.Store.Targets(ctx, principal.UserID, principal.Admin)
		if err != nil {
			return snapshot, err
		}

		ids := make([]string, 0, len(targets))
		for _, target := range targets {
			ids = append(ids, target.ID)
		}

		samples, err := history.Store.Latest(ctx, ids)
		if err != nil {
			return snapshot, err
		}

		for _, target := range targets {
			current := MonitoredServer{ID: target.ID, Name: target.Name, SparkID: target.SparkID, SparkName: target.SparkName, CPULimit: target.CPULimit, MemoryMB: target.MemoryMB, DiskMB: target.DiskMB, Usage: unknownUsage(target.ID)}
			if sample, ok := samples[target.ID]; ok && sample.SparkID == target.SparkID {
				current.Usage = ageResourceUsage(sample.Usage, sample.ReceivedAt, snapshot.CollectedAt)
				current.ReceivedAt = sample.ReceivedAt
			}

			if s.Games != nil {
				fresh := current.Usage.State == "running" && current.Usage.SampledAt > 0 && current.Usage.ContainerError == "" && current.Usage.SampleAgeSeconds <= max(15, 3*snapshot.SampleIntervalSeconds)
				if s.Sparks.connectivity == nil || !s.Sparks.connectivity.Connected(target.SparkID) {
					fresh = false
				}

				current.Game = s.Games.Snapshot(target.ID, target.SparkID, fresh, snapshot.CollectedAt)
			}

			snapshot.Servers = append(snapshot.Servers, current)
		}
	}

	if principal.Admin && principal.Allows("sparks.manage") {
		nodes, err := s.Sparks.List(ctx)
		if err != nil {
			return snapshot, err
		}

		ids := make([]string, 0, len(nodes))
		for _, node := range nodes {
			ids = append(ids, node.ID)
		}

		samples, err := history.Store.LatestSparks(ctx, ids)
		if err != nil {
			return snapshot, err
		}

		for _, node := range nodes {
			usage := unknownUsage("")
			if sample, ok := samples[node.ID]; ok && node.Connected && !node.Disabled {
				usage = ageResourceUsage(sample.Usage, sample.ReceivedAt, snapshot.CollectedAt)
			}

			snapshot.Sparks = append(snapshot.Sparks, MonitoredSpark{Spark: node, Usage: usage})
		}
	}

	return snapshot, nil
}
