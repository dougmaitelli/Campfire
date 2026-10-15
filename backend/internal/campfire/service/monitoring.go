package service

import (
	"context"
	"errors"
	"math"
	"slices"
	"sync"
	"time"

	"campfire/internal/shared/model"
)

// MonitoredServer combines authorized server metadata and measured usage.
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
}

// MonitoringSnapshot omits node inventory entirely for non-administrators.
type MonitoringSnapshot struct {
	CollectedAt int64             `json:"collected_at"`
	Servers     []MonitoredServer `json:"servers"`
	Sparks      []model.Spark     `json:"sparks,omitempty"`
	Incomplete  bool              `json:"incomplete"`
}

func unknownUsage(id string) model.ResourceUsage {
	return model.ResourceUsage{ServerID: id, State: "unknown", ContainerError: "Spark, Docker or usage sampling unavailable.", DiskError: "Game-data size unavailable."}
}

func validUsage(usage model.ResourceUsage) bool {
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

// Metrics reads only the saved server placement and never installs or delivers configuration.
func (s *DeploymentService) Metrics(ctx context.Context, id string) (model.ResourceUsage, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	s.mu.Lock()
	if s.metricSlots == nil {
		s.metricSlots = make(chan struct{}, 6)
	}

	slots := s.metricSlots
	s.mu.Unlock()

	select {
	case slots <- struct{}{}:
		defer func() { <-slots }()
	case <-ctx.Done():
		return model.ResourceUsage{}, ctx.Err()
	}

	server, err := s.Servers.Get(ctx, id)
	if err != nil {
		return model.ResourceUsage{}, err
	}

	node, err := s.Sparks.Get(ctx, server.SparkID)
	if err != nil {
		return model.ResourceUsage{}, err
	}

	if node.Disabled {
		return model.ResourceUsage{}, ErrDeploymentUnavailable
	}

	dispatch, ok := s.Remote.(RuntimeDispatcher)
	if !ok {
		return model.ResourceUsage{}, ErrDeploymentUnavailable
	}

	status, err := dispatch.Action(ctx, node.ID, model.RuntimeRequest{ServerID: id, Action: "metrics"})
	if err != nil {
		return model.ResourceUsage{}, errors.Join(ErrDeploymentUnavailable, err)
	}

	if status.ServerID != id || status.Metrics == nil || status.Metrics.ServerID != id || !validUsage(*status.Metrics) {
		return model.ResourceUsage{}, ErrDeploymentUnavailable
	}

	return *status.Metrics, nil
}

// Monitoring filters access before contacting Sparks and bounds concurrent node requests.
func (s *DeploymentService) Monitoring(ctx context.Context, principal Principal, access *AccessService) (MonitoringSnapshot, error) {
	servers, err := s.Servers.List(ctx)
	if err != nil {
		return MonitoringSnapshot{}, err
	}

	snapshot := MonitoringSnapshot{CollectedAt: time.Now().Unix(), Servers: []MonitoredServer{}}

	for _, server := range servers {
		permissions, err := access.ServerPermissions(ctx, principal, server.ID)
		if err != nil {
			return MonitoringSnapshot{}, err
		}

		if !slices.Contains(permissions, "server.view") {
			continue
		}

		snapshot.Servers = append(snapshot.Servers, MonitoredServer{ID: server.ID, Name: server.Name, SparkID: server.SparkID, SparkName: server.SparkName, CPULimit: server.CPULimit, MemoryMB: server.MemoryMB, DiskMB: server.DiskMB, Usage: unknownUsage(server.ID)})
	}

	if principal.Admin {
		snapshot.Sparks, err = s.Sparks.List(ctx)
		if err != nil {
			return MonitoringSnapshot{}, err
		}
	}

	var wg sync.WaitGroup

	jobs := make(chan int)

	for range min(6, len(snapshot.Servers)) {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for index := range jobs {
				usage, err := s.Metrics(ctx, snapshot.Servers[index].ID)
				if err == nil {
					snapshot.Servers[index].Usage = usage
					snapshot.Servers[index].ReceivedAt = time.Now().Unix()
				}
			}
		}()
	}

queueLoop:
	for index := range snapshot.Servers {
		select {
		case jobs <- index:
		case <-ctx.Done():
			snapshot.Incomplete = true
			break queueLoop
		}
	}

	close(jobs)
	wg.Wait()

	if ctx.Err() != nil {
		snapshot.Incomplete = true
	}

	snapshot.CollectedAt = time.Now().Unix()
	for index := range snapshot.Servers {
		if snapshot.Servers[index].ReceivedAt != 0 {
			delay := snapshot.CollectedAt - snapshot.Servers[index].ReceivedAt
			snapshot.Servers[index].Usage.SampleAgeSeconds += delay
			snapshot.Servers[index].Usage.DiskAgeSeconds += delay
		}
	}

	return snapshot, nil
}
