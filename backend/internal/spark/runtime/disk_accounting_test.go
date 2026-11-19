package runtime

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"campfire/internal/shared/model"
)

func TestProjectDiskUsageUsesByteChargeWithoutChangingLimits(t *testing.T) {
	for _, allocation := range []int64{0, 8} {
		q, kernel, directory := quotaFixture(t)
		if err := q.Prepare(directory, allocation); err != nil {
			t.Fatal(err)
		}

		var id uint32
		for _, attrs := range kernel.attrs {
			id = attrs.Project
		}

		limit := kernel.limits[id]
		limit.Space = 8193 // Already bytes, not the 1 KiB hard-limit units.
		kernel.limits[id] = limit

		if got, err := q.DiskUsage(directory); err != nil || got != 8193 {
			t.Fatal(got, err)
		}

		if kernel.limits[id] != limit {
			t.Fatal("sampling changed quota")
		}

		limit.Space = math.MaxUint64
		kernel.limits[id] = limit

		if _, err := q.DiskUsage(directory); err == nil {
			t.Fatal("overflow accepted")
		}

		delete(kernel.limits, id)

		if _, err := q.DiskUsage(directory); !errors.Is(err, os.ErrNotExist) || errors.Is(err, errProjectAccountingUnavailable) {
			t.Fatal("lost project query failure", err)
		}
	}
}

func TestProjectDiskUsageRejectsUnownedAndUnpreparedDirectories(t *testing.T) {
	q, kernel, directory := quotaFixture(t)
	if _, err := q.DiskUsage(directory); !errors.Is(err, errProjectAccountingUnavailable) {
		t.Fatal(err)
	}

	if err := q.Prepare(directory, 8); err != nil {
		t.Fatal(err)
	}

	for _, attrs := range kernel.attrs {
		if err := os.Remove(q.owner(attrs.Project)); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := q.DiskUsage(directory); err == nil || errors.Is(err, errProjectAccountingUnavailable) {
		t.Fatal("unowned project allowed fallback", err)
	}

	link := filepath.Join(filepath.Dir(directory), "link")
	if err := os.Symlink(directory, link); err != nil {
		t.Fatal(err)
	}

	if _, err := q.DiskUsage(link); err == nil {
		t.Fatal("followed data symlink")
	}
}

type accountingQuota struct {
	DiskQuota
	bytes int64
	err   error
	reads int
}

func (q *accountingQuota) DiskUsage(string) (int64, error) {
	q.reads++
	return q.bytes, q.err
}

func TestNativeDiskSamplingCacheFailureAndRecovery(t *testing.T) {
	m, d := fileTestManager(t)
	provider := &accountingQuota{DiskQuota: m.Quota, bytes: 8192}

	m.Quota = provider
	if err := os.WriteFile(filepath.Join(m.root(d.ServerID), "data", "save"), []byte("logical"), 0600); err != nil {
		t.Fatal(err)
	}

	usage, err := m.Measure(t.Context(), d)
	if err != nil || usage.DiskBytes == nil || *usage.DiskBytes != 8192 || usage.DiskAccounting != model.DiskAccountingProject {
		t.Fatal(usage, err)
	}

	if _, err := m.Measure(t.Context(), d); err != nil || provider.reads != 1 {
		t.Fatal("disk cache bypassed", provider.reads, err)
	}

	entry := m.metricEntry(d.ServerID)
	entry.diskAt = time.Now().Add(-time.Minute)
	entry.diskSampled = time.Now().Add(-time.Minute)
	provider.err = errors.New("private kernel detail")

	failed, err := m.Measure(t.Context(), d)
	if err != nil || failed.DiskError == "" || strings.Contains(failed.DiskError, "private") || failed.DiskBytes == nil || *failed.DiskBytes != 8192 || failed.DiskSampledAt != usage.DiskSampledAt || failed.DiskAgeSeconds < 60 || failed.DiskAccounting != model.DiskAccountingProject {
		t.Fatal("failed counter became a fresh scan", failed, err)
	}

	entry.diskAt = time.Now().Add(-time.Minute)
	provider.err, provider.bytes = nil, 16384

	recovered, err := m.Measure(t.Context(), d)
	if err != nil || recovered.DiskError != "" || recovered.DiskBytes == nil || *recovered.DiskBytes != 16384 || recovered.DiskAgeSeconds != 0 {
		t.Fatal(recovered, err)
	}
}

func TestManagedProjectMetricsDoNotWalkGameData(t *testing.T) {
	m, d := fileTestManager(t)
	q, kernel, _ := quotaFixture(t)

	volume, err := m.volume(d.ServerID)
	if err != nil {
		t.Fatal(err)
	}

	if err := q.Prepare(volume, 8); err != nil {
		t.Fatal(err)
	}

	for _, attrs := range kernel.attrs {
		limit := kernel.limits[attrs.Project]
		limit.Space = 8192
		kernel.limits[attrs.Project] = limit
	}

	if err := os.MkdirAll(filepath.Join(volume, strings.Repeat("deep/", 130)), 0700); err != nil {
		t.Fatal(err)
	}

	if _, err := measureData(t.Context(), m.root(d.ServerID)); err == nil {
		t.Fatal("fixture does not exceed scan bounds")
	}

	m.Quota = q

	bytes, method, err := m.measureDisk(t.Context(), d.ServerID)
	if err != nil || bytes != 8192 || method != model.DiskAccountingProject {
		t.Fatal("native sampling traversed game data", bytes, method, err)
	}
}

func TestDiskAccountingFallbackAndCancellation(t *testing.T) {
	m, d := fileTestManager(t)
	if err := os.WriteFile(filepath.Join(m.root(d.ServerID), "data", "save"), []byte("logical"), 0600); err != nil {
		t.Fatal(err)
	}

	for _, quota := range []DiskQuota{nil, fakeQuota{}, &accountingQuota{DiskQuota: fakeQuota{}, err: errProjectAccountingUnavailable}} {
		m.Quota = quota

		bytes, method, err := m.measureDisk(t.Context(), d.ServerID)
		if err != nil || bytes != 7 || method != model.DiskAccountingLogical {
			t.Fatal(bytes, method, err)
		}
	}

	for _, failure := range []error{syscall.EPERM, os.ErrNotExist, syscall.EIO} {
		provider := &accountingQuota{DiskQuota: fakeQuota{}, err: failure}

		m.Quota = provider
		if _, _, err := m.measureDisk(t.Context(), d.ServerID); !errors.Is(err, failure) {
			t.Fatal("native failure triggered fallback", err)
		}

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		if _, _, err := m.measureDisk(ctx, d.ServerID); !errors.Is(err, context.Canceled) || provider.reads != 1 {
			t.Fatal("cancelled sampling read the counter", err, provider.reads)
		}
	}
}
