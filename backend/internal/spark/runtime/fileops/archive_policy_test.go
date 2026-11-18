package fileops

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"campfire/internal/shared/model"
)

func TestArchiveMembersShareSafetyAndExpansionChecks(t *testing.T) {
	for _, limit := range []int{extractionLimit, archiveEntryLimit} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			members := newArchiveMembers(limit, 10)
			if name, err := members.accept("./", 0, fs.ModeDir); err != nil || name != "" {
				t.Fatal(name, err)
			}

			if name, err := members.accept("./nested/a", 4, 0600); err != nil || name != "nested/a" {
				t.Fatal(name, err)
			}

			if _, err := members.accept("nested/a", 4, 0600); err == nil {
				t.Fatal("normalized duplicate accepted")
			}

			if _, err := members.accept("b", 7, 0600); err == nil {
				t.Fatal("aggregate expansion limit ignored")
			}
		})
	}

	for _, member := range []struct {
		name string
		size int64
		mode fs.FileMode
	}{
		{"../outside", 0, 0600}, {"link", 0, fs.ModeSymlink}, {"directory/", 1, fs.ModeDir}, {"negative", -1, 0600},
	} {
		if _, err := newArchiveMembers(10, 10).accept(member.name, member.size, member.mode); err == nil {
			t.Fatal("unsafe member accepted", member)
		}
	}

	members := newArchiveMembers(1, 10)
	if _, err := members.accept("a", 0, 0600); err != nil {
		t.Fatal(err)
	}

	if _, err := members.accept("b", 0, 0600); err == nil {
		t.Fatal("entry limit ignored")
	}
}

func TestArchiveAllocationAccountsWritesWithoutPerMemberWalks(t *testing.T) {
	calls := 0
	used := int64(1000)

	a := &archiveAllocation{limit: 3000, usage: func(context.Context, string) (int64, error) { calls++; return used, nil }}
	for range 1000 {
		if err := a.reserve(t.Context(), "volume", 0, 1); err != nil {
			t.Fatal(err)
		}
	}

	if calls != 1 || a.used != 2000 {
		t.Fatal("allocation walked once per member or lost accounting", calls, a.used)
	}

	if err := a.reserve(t.Context(), "volume", 500, 100); err != nil || a.used != 1600 {
		t.Fatal("overwrite accounting", a.used, err)
	}

	used = 2999 // A concurrent writer has consumed space since the last snapshot.
	a.refreshed = time.Now().Add(-2 * archiveAllocationRefresh)

	var failure *model.FileError
	if err := a.reserve(t.Context(), "volume", 0, 2); !errors.As(err, &failure) || failure.Code != model.FileLimitExceeded {
		t.Fatal("reconciliation ignored external writes", err)
	}

	if calls != 2 {
		t.Fatal("expected one periodic reconciliation", calls)
	}
}

func TestArchiveAllocationPropagatesErrorsAndDoesNotScanUnlimitedServers(t *testing.T) {
	a := &archiveAllocation{usage: func(context.Context, string) (int64, error) {
		t.Fatal("unlimited allocation scanned files")
		return 0, nil
	}}
	if err := a.reserve(t.Context(), "volume", 0, 100); err != nil {
		t.Fatal(err)
	}

	a.limit = 100

	a.usage = func(context.Context, string) (int64, error) { return 0, fs.ErrPermission }
	if err := a.reserve(t.Context(), "volume", 0, 1); !errors.Is(err, fs.ErrPermission) {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := a.reserve(ctx, "volume", 0, 1); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func BenchmarkArchiveAllocationLargeExistingTree(b *testing.B) {
	volume := b.TempDir()
	for i := range 5000 {
		if err := os.WriteFile(filepath.Join(volume, fmt.Sprintf("existing-%d", i)), nil, 0600); err != nil {
			b.Fatal(err)
		}
	}

	scans := 0

	a := &archiveAllocation{limit: 1 << 30, usage: func(ctx context.Context, volume string) (int64, error) { scans++; return FileUsage(ctx, volume) }}
	if err := a.reserve(b.Context(), volume, 0, 0); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()

	for range b.N {
		if err := a.reserve(b.Context(), volume, 0, 1); err != nil {
			b.Fatal(err)
		}
	}

	b.StopTimer()
	b.ReportMetric(float64(scans), "tree-scans")
}
