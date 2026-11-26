//go:build linux

package runtime

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Supply a disposable ext4/XFS filesystem mounted with project quotas. Never
// enable quota features or change mount options on an existing host filesystem.
func TestLiveProjectDiskAccounting(t *testing.T) {
	root := os.Getenv("CAMPFIRE_TEST_QUOTA_ROOT")
	if root == "" {
		t.Skip("set CAMPFIRE_TEST_QUOTA_ROOT to a disposable project-quota filesystem")
	}

	base, err := os.MkdirTemp(root, "campfire-accounting-")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = os.RemoveAll(base) })

	directory := filepath.Join(base, "data")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}

	quota := &projectQuotas{registry: filepath.Join(base, "projects"), kernel: linuxProjectKernel{}}
	if err := quota.Prepare(t.Context(), directory, 64); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err := quota.Prepare(t.Context(), directory, 0); err != nil {
			t.Error(err)
		}
	})

	read := func() int64 {
		t.Helper()

		usage, err := quota.DiskUsage(t.Context(), directory)
		if err != nil {
			t.Fatal(err)
		}

		return usage
	}
	baseline := read()

	file, err := os.Create(filepath.Join(directory, "open"))
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = file.Close() }()

	if _, err := file.Write(bytes.Repeat([]byte("x"), 1<<20)); err != nil {
		t.Fatal(err)
	}

	if err := file.Sync(); err != nil {
		t.Fatal(err)
	}

	charged := read()
	if charged < baseline+(1<<20) {
		t.Fatal("allocated data was not charged in bytes", baseline, charged)
	}

	if err := os.WriteFile(filepath.Join(directory, "sparse"), nil, 0600); err != nil {
		t.Fatal(err)
	}

	if err := os.Truncate(filepath.Join(directory, "sparse"), 16<<20); err != nil {
		t.Fatal(err)
	}

	logical, err := measureData(t.Context(), base)
	if err != nil || logical <= read() {
		t.Fatal("quota accounting incorrectly follows logical sparse size", logical, err)
	}

	if err := os.Remove(file.Name()); err != nil {
		t.Fatal(err)
	}

	if got := read(); got < charged {
		t.Fatal("deleted-open data disappeared from quota charge", got, charged)
	}

	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for read() >= charged {
		if time.Now().After(deadline) {
			t.Fatal("closed deleted data remained charged")
		}

		time.Sleep(10 * time.Millisecond)
	}

	if err := quota.Prepare(t.Context(), directory, 0); err != nil {
		t.Fatal(err)
	}

	if _, err := quota.DiskUsage(t.Context(), directory); err != nil {
		t.Fatal("unlimited project lost accounting", err)
	}
}
