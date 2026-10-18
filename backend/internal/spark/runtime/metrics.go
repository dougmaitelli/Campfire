package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"path"
	"strings"
	"syscall"
	"time"

	"campfire/internal/shared/model"
)

// ContainerSample is an unrounded Docker counters snapshot, injectable in tests.
type ContainerSample struct {
	ID          string      `json:"id"`
	CPU         cpuCounters `json:"cpu_stats"`
	PreviousCPU cpuCounters `json:"precpu_stats"`
	Memory      struct {
		Usage *uint64           `json:"usage"`
		Stats map[string]uint64 `json:"stats"`
	} `json:"memory_stats"`
}

type cpuCounters struct {
	Usage struct {
		Total  uint64   `json:"total_usage"`
		PerCPU []uint64 `json:"percpu_usage"`
	} `json:"cpu_usage"`
	System uint64 `json:"system_cpu_usage"`
	Online uint64 `json:"online_cpus"`
}

// Stats reads one raw Linux Docker sample through the same Unix socket as console input.
func (Docker) Stats(ctx context.Context, name string) (ContainerSample, error) {
	host := os.Getenv("DOCKER_HOST")
	if host == "" {
		host = "unix:///var/run/docker.sock"
	}

	if !strings.HasPrefix(host, "unix://") || os.Getenv("DOCKER_CONTEXT") != "" {
		return ContainerSample{}, errors.New("usage reporting requires the local Docker Unix socket")
	}

	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", strings.TrimPrefix(host, "unix://"))
	}, DisableKeepAlives: true}
	defer transport.CloseIdleConnections()

	return readContainerSample(ctx, &http.Client{Transport: transport, Timeout: 3 * time.Second}, name)
}

func readContainerSample(ctx context.Context, client *http.Client, name string) (ContainerSample, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker/containers/"+name+"/stats?stream=false&one-shot=true", nil)
	if err != nil {
		return ContainerSample{}, err
	}

	response, err := client.Do(request)
	if err != nil {
		return ContainerSample{}, errors.New("Docker usage sample unavailable")
	}

	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		return ContainerSample{}, errors.New("Docker usage sample unavailable")
	}

	data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return ContainerSample{}, errors.New("invalid Docker usage sample")
	}

	var sample ContainerSample
	if err := json.Unmarshal(data, &sample); err != nil || sample.ID == "" {
		return ContainerSample{}, errors.New("invalid Docker usage sample")
	}

	return sample, nil
}

type metricsEntry struct {
	gate             chan struct{}
	usage            model.ResourceUsage
	containerAt      time.Time
	diskAt           time.Time
	previous         ContainerSample
	previousAt       time.Time
	containerSampled time.Time
	diskSampled      time.Time
}

func (m *Manager) metricEntry(id string) *metricsEntry {
	m.metricsMu.Lock()
	defer m.metricsMu.Unlock()

	if m.metrics == nil {
		m.metrics = make(map[string]*metricsEntry)
	}

	if m.metrics[id] == nil {
		m.metrics[id] = &metricsEntry{gate: make(chan struct{}, 1), usage: model.ResourceUsage{ServerID: id, State: "unknown"}}
	}

	return m.metrics[id]
}

// Measure samples usage independently of lifecycle locks and coalesces concurrent readers.
func (m *Manager) Measure(ctx context.Context, d model.Deployment) (model.ResourceUsage, error) {
	if !safeID.MatchString(d.ServerID) {
		return model.ResourceUsage{}, errors.New("invalid server identity")
	}

	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	entry := m.metricEntry(d.ServerID)
	select {
	case entry.gate <- struct{}{}:
		defer func() { <-entry.gate }()
	case <-ctx.Done():
		return model.ResourceUsage{}, ctx.Err()
	}

	state, err := m.load(d.ServerID)
	if err != nil {
		return model.ResourceUsage{}, err
	}

	if !state.mu.TryLock() {
		usage := entry.snapshot()
		usage.ContainerError = "Runtime status is busy; usage may be stale."
		usage.DiskQuotaEnforced = false
		usage.DiskQuotaError = "Runtime status is busy; quota verification is unavailable."

		return usage, nil
	}
	installed, busy, phase, operation := state.Installed, state.Status.Busy, state.Status.State, state.Status.Operation
	state.mu.Unlock()

	now := time.Now()
	if now.Sub(entry.containerAt) >= time.Second {
		entry.containerAt = now
		entry.usage.CPUPercent, entry.usage.MemoryBytes = nil, nil
		entry.usage.ContainerError = ""
		entry.usage.SampledAt = 0

		name := container(d.ServerID)
		if busy && operation == "install" {
			name += "-install"
		}

		out, inspectErr := m.Docker.Run(ctx, nil, "inspect", "--format", "{{.State.Status}}", name)

		actual := strings.TrimSpace(out)
		if inspectErr != nil && (strings.Contains(strings.ToLower(out), "no such object") || strings.Contains(strings.ToLower(out), "no such container")) {
			actual = "missing"
			inspectErr = nil
		}

		entry.usage.State = actual
		if inspectErr != nil {
			entry.usage.State = "unknown"
			entry.usage.ContainerError = "Docker state unavailable."
		} else if actual == "running" || actual == "paused" {
			provider, ok := m.Docker.(interface {
				Stats(context.Context, string) (ContainerSample, error)
			})
			if !ok {
				entry.usage.ContainerError = "Docker usage sampling unavailable."
			} else {
				sample, sampleErr := provider.Stats(ctx, name)
				if sampleErr != nil {
					entry.usage.ContainerError = "Docker usage sample unavailable."
				} else {
					previous := sample.PreviousCPU
					if previous.System == 0 && entry.previous.ID == sample.ID && now.Sub(entry.previousAt) <= 30*time.Second {
						previous = entry.previous.CPU
					}

					entry.usage.CPUPercent = cpuPercent(sample.CPU, previous)
					entry.usage.MemoryBytes = memoryBytes(sample)
					entry.usage.SampledAt = now.Unix()
					entry.containerSampled = now
					entry.previous, entry.previousAt = sample, now
				}
			}
		} else if actual == "exited" || actual == "created" || actual == "missing" || actual == "dead" {
			zeroCPU, zeroMemory := float64(0), int64(0)
			entry.usage.CPUPercent, entry.usage.MemoryBytes = &zeroCPU, &zeroMemory
			entry.usage.SampledAt = now.Unix()
			entry.containerSampled = now
			entry.previous = ContainerSample{}
		} else {
			entry.usage.State = "unknown"
			entry.usage.ContainerError = "Docker state unavailable."
		}

		if actual == "exited" || actual == "created" || actual == "dead" {
			entry.usage.State = "stopped"
		}

		if actual == "missing" {
			entry.usage.State = "not_installed"
			if installed {
				entry.usage.State = "stopped"
			}

			if phase == "deleted" {
				entry.usage.State = "deleted"
			}
		}

		if busy {
			entry.usage.State = phase
		}
	}

	if now.Sub(entry.diskAt) >= 30*time.Second {
		entry.diskAt = now
		diskCtx, stop := context.WithTimeout(ctx, 2*time.Second)
		bytes, diskErr := measureData(diskCtx, m.root(d.ServerID))

		stop()

		entry.usage.DiskError = ""
		if diskErr != nil {
			entry.usage.DiskError = "Game-data size unavailable; last successful reading may be stale."
		} else {
			entry.usage.DiskBytes = &bytes
			entry.diskSampled = time.Now()
			entry.usage.DiskSampledAt = entry.diskSampled.Unix()
		}
	}

	entry.usage.DiskQuotaEnforced = false
	entry.usage.DiskQuotaError = ""

	if volume, err := m.volume(d.ServerID); err == nil && m.Quota != nil && d.Configuration.DiskMB > 0 {
		if err := m.Quota.Verify(volume, d.Configuration.DiskMB); err != nil {
			entry.usage.DiskQuotaError = "The configured game-data quota could not be verified. Check host project quotas and apply the allocation by starting the server."
		} else {
			entry.usage.DiskQuotaEnforced = true
			entry.usage.DiskQuotaError = ""
		}
	}

	return entry.snapshot(), nil
}

func (entry *metricsEntry) snapshot() model.ResourceUsage {
	usage := entry.usage
	if !entry.containerSampled.IsZero() {
		usage.SampleAgeSeconds = int64(time.Since(entry.containerSampled).Seconds())
	}

	if !entry.diskSampled.IsZero() {
		usage.DiskAgeSeconds = int64(time.Since(entry.diskSampled).Seconds())
	}

	return usage
}

func cpuPercent(current, previous cpuCounters) *float64 {
	cpus := current.Online
	if cpus == 0 {
		cpus = uint64(len(current.Usage.PerCPU))
	}

	if previous.System == 0 || cpus == 0 || current.System <= previous.System || current.Usage.Total < previous.Usage.Total {
		return nil
	}

	value := float64(current.Usage.Total-previous.Usage.Total) / float64(current.System-previous.System) * float64(cpus) * 100
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return nil
	}

	return &value
}

func memoryBytes(sample ContainerSample) *int64 {
	if sample.Memory.Usage == nil || *sample.Memory.Usage > math.MaxInt64 {
		return nil
	}

	usage := *sample.Memory.Usage

	cache, ok := sample.Memory.Stats["total_inactive_file"]
	if !ok {
		cache = sample.Memory.Stats["inactive_file"]
	}

	if cache <= usage {
		usage -= cache
	}

	value := int64(usage)

	return &value
}

type inodeKey struct{ device, inode uint64 }

func measureData(ctx context.Context, directory string) (int64, error) {
	parent, err := os.OpenRoot(directory)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}

	if err != nil {
		return 0, err
	}

	defer func() { _ = parent.Close() }()

	info, err := parent.Lstat("data")
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}

	if err != nil || !info.IsDir() {
		return 0, errors.New("game data is not a directory")
	}

	root, err := parent.OpenRoot("data")
	if err != nil {
		return 0, err
	}

	defer func() { _ = root.Close() }()

	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		return 0, errors.New("game data changed during sampling")
	}

	var total int64

	var count int

	seen := make(map[inodeKey]bool)

	var walk func(string, int) error

	walk = func(name string, depth int) error {
		if depth > 128 {
			return errors.New("game-data sample exceeds depth bound")
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		directory, err := root.OpenFile(name, os.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if err != nil {
			return err
		}

		defer func() { _ = directory.Close() }()

		for {
			children, readErr := directory.ReadDir(128)
			if readErr != nil && !errors.Is(readErr, io.EOF) {
				return readErr
			}

			for _, child := range children {
				count++
				if count > 100000 {
					return errors.New("game-data sample exceeds entry bound")
				}

				if err := ctx.Err(); err != nil {
					return err
				}

				childName := path.Join(name, child.Name())

				info, err := root.Lstat(childName)
				if err != nil {
					return err
				}

				if !info.IsDir() && !info.Mode().IsRegular() {
					continue
				}

				if stat, ok := info.Sys().(*syscall.Stat_t); ok {
					key := inodeKey{stat.Dev, stat.Ino}
					if seen[key] {
						continue
					}

					seen[key] = true
				}

				if info.IsDir() {
					if err := walk(childName, depth+1); err != nil {
						return err
					}
				} else {
					if info.Size() < 0 || info.Size() > math.MaxInt64-total {
						return errors.New("invalid game-data size")
					}

					total += info.Size()
				}
			}

			if errors.Is(readErr, io.EOF) || len(children) == 0 {
				return nil
			}
		}
	}
	err = walk(".", 0)

	return total, err
}
