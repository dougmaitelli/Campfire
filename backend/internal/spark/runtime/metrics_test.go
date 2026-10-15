package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func sample(t *testing.T, id string, total, system uint64) ContainerSample {
	t.Helper()

	var result ContainerSample
	if err := json.Unmarshal([]byte(`{"id":"`+id+`","memory_stats":{"usage":1048576,"stats":{"inactive_file":262144}}}`), &result); err != nil {
		t.Fatal(err)
	}

	result.CPU.System = system
	result.CPU.Online = 4
	result.CPU.Usage.Total = total

	return result
}

type metricsDocker struct {
	*fakeDocker
	statsMu sync.Mutex
	samples []ContainerSample
	calls   int
	fail    bool
}

func (d *metricsDocker) Stats(context.Context, string) (ContainerSample, error) {
	d.statsMu.Lock()
	defer d.statsMu.Unlock()

	d.calls++
	if d.fail {
		return ContainerSample{}, errors.New("private-docker-error")
	}

	return d.samples[min(d.calls-1, len(d.samples)-1)], nil
}

func TestMetricsCountersAndMemoryCache(t *testing.T) {
	for _, tc := range []struct {
		name              string
		current, previous cpuCounters
		want              *float64
	}{
		{"idle", sample(t, "a", 100, 2000).CPU, sample(t, "a", 100, 1000).CPU, new(float64(0))},
		{"multiple cores", sample(t, "a", 900, 2000).CPU, sample(t, "a", 100, 1000).CPU, new(float64(320))},
		{"first sample", sample(t, "a", 100, 1000).CPU, cpuCounters{}, nil},
		{"reset", sample(t, "a", 50, 2000).CPU, sample(t, "a", 100, 1000).CPU, nil},
		{"zero system delta", sample(t, "a", 200, 1000).CPU, sample(t, "a", 100, 1000).CPU, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := cpuPercent(tc.current, tc.previous)
			if tc.want == nil {
				if got != nil {
					t.Fatal("unknown CPU reported as measured", *got)
				}
			} else if got == nil || math.Abs(*got-*tc.want) > 0.00001 {
				t.Fatal("incorrect core normalization", got, tc.want)
			}
		})
	}

	current := sample(t, "a", 200, 2000)
	current.CPU.Online = 0

	current.CPU.Usage.PerCPU = []uint64{1, 1}
	if got := cpuPercent(current.CPU, sample(t, "a", 100, 1000).CPU); got == nil || *got != 20 {
		t.Fatal("per-CPU fallback incorrect", got)
	}

	for _, cache := range []string{"total_inactive_file", "inactive_file"} {
		current.Memory.Stats = map[string]uint64{cache: 262144}
		if got := memoryBytes(current); got == nil || *got != 786432 {
			t.Fatal("cache not excluded", cache, got)
		}
	}

	current.Memory.Stats = map[string]uint64{"inactive_file": 1 << 30}
	if got := memoryBytes(current); got == nil || *got != 1048576 {
		t.Fatal("cache larger than usage underflowed", got)
	}

	current.Memory.Usage = nil
	if got := memoryBytes(current); got != nil {
		t.Fatal("missing memory treated as zero")
	}
}

func TestMetricsSamplingCachesAndPreservesUnknowns(t *testing.T) {
	ctx := context.Background()
	d := runtimeFixture()

	m := New(ctx, t.TempDir())
	defer m.Close()

	docker := &metricsDocker{fakeDocker: &fakeDocker{states: map[string]string{container(d.ServerID): "running"}}, samples: []ContainerSample{sample(t, "first", 100, 1000), sample(t, "first", 200, 2000), sample(t, "replacement", 5, 3000)}}
	m.Docker = docker

	data := filepath.Join(m.root(d.ServerID), "data")
	if err := os.MkdirAll(data, 0700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(data, "save"), []byte("actual-game-data"), 0600); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()

			usage, err := m.Measure(ctx, d)
			if err != nil {
				t.Error(err)
				return
			}

			if usage.CPUPercent != nil || usage.MemoryBytes == nil || *usage.MemoryBytes != 786432 || usage.DiskBytes == nil || *usage.DiskBytes != 16 {
				t.Error("incorrect first sample", usage)
			}
		}()
	}

	wg.Wait()

	if docker.calls != 1 {
		t.Fatal("concurrent reads duplicated Docker samples", docker.calls)
	}

	entry := m.metricEntry(d.ServerID)
	entry.containerAt = time.Now().Add(-6 * time.Second)

	usage, err := m.Measure(ctx, d)
	if err != nil || usage.CPUPercent == nil || *usage.CPUPercent != 40 || usage.SampledAt == 0 || usage.DiskSampledAt == 0 {
		t.Fatal("counter delta not measured", usage, err)
	}

	entry.containerAt = time.Now().Add(-6 * time.Second)

	usage, err = m.Measure(ctx, d)
	if err != nil || usage.CPUPercent != nil {
		t.Fatal("replacement container reused old CPU baseline", usage, err)
	}

	docker.fail = true
	entry.containerAt = time.Now().Add(-6 * time.Second)
	usage, err = m.Measure(ctx, d)

	encoded, encodeErr := json.Marshal(usage)
	if err != nil || encodeErr != nil || usage.CPUPercent != nil || usage.MemoryBytes != nil || usage.ContainerError == "" || strings.Contains(string(encoded), "private-docker-error") {
		t.Fatal("failed sample became zero or leaked diagnostics", usage, err)
	}

	docker.mu.Lock()
	docker.states[container(d.ServerID)] = "exited"
	docker.mu.Unlock()

	entry.containerAt = time.Now().Add(-6 * time.Second)

	usage, err = m.Measure(ctx, d)
	if err != nil || usage.CPUPercent == nil || *usage.CPUPercent != 0 || usage.MemoryBytes == nil || *usage.MemoryBytes != 0 || usage.State != "stopped" {
		t.Fatal("confirmed stopped container did not report zero", usage, err)
	}
}

func TestMetricsDiskContainmentAndLastSuccessfulReading(t *testing.T) {
	ctx := context.Background()
	directory := t.TempDir()

	data := filepath.Join(directory, "data")
	if err := os.MkdirAll(filepath.Join(data, "nested"), 0700); err != nil {
		t.Fatal(err)
	}

	file := filepath.Join(data, "nested", "save")
	if err := os.WriteFile(file, []byte("measured"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := os.Link(file, filepath.Join(data, "hardlink")); err != nil {
		t.Fatal(err)
	}

	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "private"), make([]byte, 1024), 0600); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink(outside, filepath.Join(data, "escape")); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(directory, "backups"), 0700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(directory, "backups", "archive"), make([]byte, 4096), 0600); err != nil {
		t.Fatal(err)
	}

	if got, err := measureData(ctx, directory); err != nil || got != 8 {
		t.Fatal("scan counted links or backups", got, err)
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()

	if _, err := measureData(cancelled, directory); !errors.Is(err, context.Canceled) {
		t.Fatal("scan ignored cancellation", err)
	}

	m := New(ctx, t.TempDir())
	defer m.Close()

	d := runtimeFixture()

	m.Docker = &fakeDocker{states: map[string]string{}}
	if err := os.MkdirAll(m.root(d.ServerID), 0700); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink(outside, filepath.Join(m.root(d.ServerID), "data")); err != nil {
		t.Fatal(err)
	}

	entry := m.metricEntry(d.ServerID)
	entry.usage.DiskBytes = new(int64(42))
	entry.usage.DiskSampledAt = time.Now().Add(-time.Minute).Unix()
	entry.diskSampled = time.Now().Add(-time.Minute)

	usage, err := m.Measure(ctx, d)
	if err != nil || usage.DiskError == "" || usage.DiskBytes == nil || *usage.DiskBytes != 42 || usage.DiskAgeSeconds < 60 {
		t.Fatal("failed scan discarded last reading or followed data symlink", usage, err)
	}
}

func TestMetricsDoNotWaitForLifecycleLock(t *testing.T) {
	m := New(context.Background(), t.TempDir())
	defer m.Close()

	d := runtimeFixture()

	entry, err := m.load(d.ServerID)
	if err != nil {
		t.Fatal(err)
	}

	entry.mu.Lock()
	defer entry.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	usage, err := m.Measure(ctx, d)
	if err != nil || usage.ContainerError == "" || usage.SampledAt != 0 {
		t.Fatal("busy lifecycle was treated as a fresh sample", usage, err)
	}
}

type metricRoundTripper func(*http.Request) (*http.Response, error)

func (f metricRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDockerMetricsHTTPValidation(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		fail       bool
	}{
		{"valid", `{"id":"container","cpu_stats":{"system_cpu_usage":1000},"memory_stats":{"usage":128}}`, 200, false},
		{"missing container", `private-error`, 404, true},
		{"malformed", `{"id":`, 200, true},
		{"missing identity", `{}`, 200, true},
		{"oversized", strings.Repeat(" ", (1<<20)+1), 200, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: metricRoundTripper(func(r *http.Request) (*http.Response, error) {
				if r.Method != http.MethodGet || r.URL.Path != "/containers/campfire-game/stats" || r.URL.Query().Get("stream") != "false" || r.URL.Query().Get("one-shot") != "true" {
					t.Error("unexpected Docker stats request", r.URL)
				}

				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
			})}

			got, err := readContainerSample(context.Background(), client, "campfire-game")
			if tc.fail {
				if err == nil || strings.Contains(err.Error(), "private-error") {
					t.Fatal("invalid response accepted or leaked", err)
				}
			} else if err != nil || got.Memory.Usage == nil || *got.Memory.Usage != 128 {
				t.Fatal(got, err)
			}
		})
	}
}
