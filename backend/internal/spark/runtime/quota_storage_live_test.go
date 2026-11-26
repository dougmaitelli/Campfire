//go:build linux

package runtime

import (
	"crypto/rand"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"campfire/internal/shared/model"

	"golang.org/x/sys/unix"
)

// Run under Spark's service capabilities so writes from UID 0 obey hard quotas.
func TestLiveSparkUploadQuota(t *testing.T) {
	if os.Getenv("CAMPFIRE_TEST_STORAGE_ROOT") == "" {
		t.Skip("set CAMPFIRE_TEST_STORAGE_ROOT to a disposable quota-enabled filesystem")
	}

	header := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}

	var caps [2]unix.CapUserData
	if err := unix.Capget(&header, &caps[0]); err != nil {
		t.Fatal(err)
	}

	if caps[0].Effective&(1<<unix.CAP_SYS_RESOURCE) != 0 {
		t.Skip("run the test with Spark's service setting CapabilityBoundingSet=~CAP_SYS_RESOURCE")
	}

	d := runtimeFixture()
	d.Configuration.DiskMB = 8
	m := sparseTestManager(t, d)

	m.Ownership = func(string, string) error { return nil }
	if s := awaitOperation(t, m, d, "install", "install"); s.OperationState != model.OperationSucceeded {
		t.Fatal(s)
	}

	dir, err := openQuotaDir(filepath.Join(m.root(d.ServerID), "data"))
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = dir.Close() }()

	q := model.FileRequest{ServerID: d.ServerID, Operation: model.FileOperationUploadBegin, Path: "upload", Size: -1, Revision: "absent"}

	start, err := m.Files(t.Context(), d, q)
	if err != nil {
		t.Fatal(err)
	}

	q.Operation, q.UploadID, q.Data = model.FileOperationUploadChunk, start.UploadID, make([]byte, model.FileLimit)
	for range 32 {
		_, _ = rand.Read(q.Data)
		_, err := m.Files(t.Context(), d, q)

		var failure *model.FileError
		if errors.As(err, &failure) && failure.Code == model.FileLimitExceeded {
			return
		}

		if err != nil {
			t.Fatal(err)
		}

		if err := unix.Syncfs(int(dir.Fd())); err != nil {
			t.Fatal(err)
		}

		q.Offset += int64(len(q.Data))
	}

	t.Fatal("Spark accepted a 32 MiB upload under an 8 MiB allocation")
}

// Run as root on a disposable, quota-enabled filesystem. The test works with
// each production provider and writes at most 32 MiB of incompressible data.
func TestLiveStorageQuota(t *testing.T) {
	root := os.Getenv("CAMPFIRE_TEST_STORAGE_ROOT")
	if root == "" {
		t.Skip("set CAMPFIRE_TEST_STORAGE_ROOT to a disposable quota-enabled filesystem")
	}

	base, err := os.MkdirTemp(root, "spark-storage-")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err := os.RemoveAll(base); err != nil {
			t.Error(err)
		}
	})

	q, err := detectDiskQuota(t.Context(), base)
	if err != nil {
		t.Fatal(err)
	}

	stage := filepath.Join(base, ".restore-stage")
	if err := os.Mkdir(stage, 0700); err != nil {
		t.Fatal(err)
	}

	if _, ok := q.(*btrfsQuotas); ok {
		before, err := os.Stat(stage)
		if err != nil {
			t.Fatal(err)
		}

		m := &Manager{Quota: q}
		if err := m.resizeMountedQuota(t.Context(), stage, 4); err == nil {
			t.Fatal("replaced a potentially mounted directory")
		}

		after, err := os.Stat(stage)
		if err != nil || !os.SameFile(before, after) {
			t.Fatal("original directory was lost", err)
		}
	}

	if err := q.Prepare(t.Context(), stage, 4); err != nil {
		t.Fatal(err)
	}

	dir := filepath.Join(base, "data")
	if err := os.Rename(stage, dir); err != nil {
		t.Fatal(err)
	}

	if err := q.Verify(t.Context(), dir, 4); err != nil {
		t.Fatal("quota lost after rename", err)
	}

	if err := q.Prepare(t.Context(), dir, 8); err != nil {
		t.Fatal(err)
	}

	f, err := os.Create(filepath.Join(dir, "game"))
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = f.Close() }()

	var exhausted bool

	for range 32 {
		cmd := exec.CommandContext(t.Context(), "dd", "if=/dev/urandom", "bs=1048576", "count=1")
		cmd.Stdout = f
		cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 65534, Gid: 65534}}

		var stderr strings.Builder

		cmd.Stderr = &stderr
		err := cmd.Run()

		syncErr := unix.Syncfs(int(f.Fd()))
		if strings.Contains(strings.ToLower(stderr.String()), "quota exceeded") || errors.Is(syncErr, unix.EDQUOT) {
			exhausted = true
			break
		}

		if err != nil || syncErr != nil {
			t.Fatalf("write: %v, sync: %v, %s", err, syncErr, &stderr)
		}
	}

	if !exhausted {
		t.Fatal("filesystem accepted 32 MiB under an 8 MiB quota")
	}

	restarted, err := detectDiskQuota(t.Context(), base)
	if err != nil {
		t.Fatal(err)
	}

	if err := restarted.Verify(t.Context(), dir, 8); err != nil {
		t.Fatal(err)
	}

	provider, ok := restarted.(diskUsageProvider)
	if !ok {
		t.Fatal("missing native usage counter")
	}

	if used, err := provider.DiskUsage(t.Context(), dir); err != nil || used < 1<<20 {
		t.Fatal("missing native space charge", used, err)
	}

	if err := restarted.Prepare(t.Context(), dir, 1); err == nil {
		t.Fatal("shrunk below charged usage")
	}

	if err := restarted.Prepare(t.Context(), dir, 0); err != nil {
		t.Fatal(err)
	}

	if err := restarted.Verify(t.Context(), dir, 0); err != nil {
		t.Fatal(err)
	}
}
