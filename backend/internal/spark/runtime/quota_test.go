package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"campfire/internal/shared/model"
)

type quotaKernelFake struct {
	attrs     map[uint64]projectAttributes
	limits    map[uint32]projectLimit
	disabled  bool
	setError  error
	ignoreSet bool
}

type recordingQuota struct {
	fakeQuota
	allocations []int64
}

func (q *recordingQuota) Prepare(_ context.Context, _ string, mb int64) error {
	q.allocations = append(q.allocations, mb)
	return nil
}

func TestUploadReconcilesQuotaOnlyOnBeginAndAllocationChange(t *testing.T) {
	m, d := fileTestManager(t)
	q := &recordingQuota{}
	m.Quota = q
	begin := model.FileRequest{ServerID: d.ServerID, Operation: model.FileOperationUploadBegin, Path: "save", Size: 3, Revision: "absent"}

	result, err := m.Files(t.Context(), d, begin)
	if err != nil {
		t.Fatal(err)
	}

	chunk := model.FileRequest{ServerID: d.ServerID, Path: "save", UploadID: result.UploadID, Operation: model.FileOperationUploadChunk, Data: []byte("x")}

	for i := range 3 {
		if i == 1 {
			d.Configuration.DiskMB++
		}

		chunk.Offset = int64(i)
		if _, err := m.Files(t.Context(), d, chunk); err != nil {
			t.Fatal(err)
		}
	}

	chunk.Operation, chunk.Data = model.FileOperationUploadCommit, nil
	if _, err := m.Files(t.Context(), d, chunk); err != nil {
		t.Fatal(err)
	}

	if len(q.allocations) != 2 || q.allocations[0] != d.Configuration.DiskMB-1 || q.allocations[1] != d.Configuration.DiskMB {
		t.Fatal(q.allocations)
	}
}

func (k *quotaKernelFake) Enabled(*os.File) error {
	if k.disabled {
		return syscall.EOPNOTSUPP
	}

	return nil
}

func inode(file *os.File) uint64 {
	var stat syscall.Stat_t
	if err := syscall.Fstat(int(file.Fd()), &stat); err != nil {
		panic(err)
	}

	return stat.Ino
}

func (k *quotaKernelFake) Attributes(f *os.File) (projectAttributes, error) {
	return k.attrs[inode(f)], nil
}

func (k *quotaKernelFake) Assign(f *os.File, attrs projectAttributes) error {
	k.attrs[inode(f)] = attrs
	return nil
}

func (k *quotaKernelFake) Get(_ context.Context, _ *os.File, id uint32) (projectLimit, error) {
	limit, ok := k.limits[id]
	if !ok {
		return limit, os.ErrNotExist
	}

	return limit, nil
}

func (k *quotaKernelFake) Set(_ context.Context, _ *os.File, id uint32, limit projectLimit) error {
	if k.setError != nil {
		return k.setError
	}

	if k.ignoreSet {
		return nil
	}

	old := k.limits[id]
	old.Hard, old.Soft = limit.Hard, limit.Soft
	k.limits[id] = old

	return nil
}

func quotaFixture(t *testing.T) (*projectQuotas, *quotaKernelFake, string) {
	t.Helper()
	root := t.TempDir()

	directory := filepath.Join(root, "data")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}

	kernel := &quotaKernelFake{attrs: make(map[uint64]projectAttributes), limits: make(map[uint32]projectLimit)}

	return &projectQuotas{registry: filepath.Join(root, ".quota-projects"), kernel: kernel}, kernel, directory
}

func TestProjectLockIsPerDirectoryAndCancelable(t *testing.T) {
	q, _, first := quotaFixture(t)

	second := t.TempDir()
	if err := q.Prepare(t.Context(), first, 8); err != nil {
		t.Fatal(err)
	}

	f, owner, err := q.open(first)
	if err != nil {
		t.Fatal(err)
	}

	_ = f.Close()

	lock, err := q.lockDirectory(t.Context(), owner)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = lock.Close() })

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	if err := q.Prepare(ctx, second, 8); err != nil {
		t.Fatalf("unrelated game waited for the locked project: %v", err)
	}

	wait, stop := context.WithTimeout(t.Context(), 40*time.Millisecond)
	defer stop()

	if err := q.Prepare(wait, first, 16); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("contended quota update ignored deadline: %v", err)
	}

	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}

	if err := q.Prepare(ctx, first, 16); err != nil {
		t.Fatal(err)
	}
}

func TestRestoreCleanupFailureIsIsolatedAndRetried(t *testing.T) {
	m, d := fileTestManager(t)
	q, kernel, _ := quotaFixture(t)
	m.Quota, m.retireProjects = nil, q

	stage := filepath.Join(m.root(d.ServerID), ".restore-pending")
	if err := os.Mkdir(stage, 0700); err != nil {
		t.Fatal(err)
	}

	if err := q.Prepare(t.Context(), stage, 8); err != nil {
		t.Fatal(err)
	}

	kernel.setError = syscall.EIO

	if err := m.WatchLogs(d); err != nil {
		t.Fatal("could not attach startup monitor", err)
	}

	status := func(deployment model.Deployment) model.RuntimeStatus {
		t.Helper()

		s, err := m.Execute(t.Context(), deployment, model.RuntimeRequest{ServerID: deployment.ServerID, Action: "status"})
		if err != nil {
			t.Fatal(err)
		}

		return s
	}
	if s := status(d); s.State != model.RuntimeStateUnknown || !strings.Contains(s.Error, errRestoreRecovery.Error()) {
		t.Fatal("missing per-server cleanup error", s)
	}

	if _, err := os.Stat(stage); err != nil {
		t.Fatal("lost the stage needed for retry", err)
	}

	other := d

	other.ServerID = "unaffected"
	if s := status(other); s.Error != "" || s.State != model.RuntimeStateNotInstalled {
		t.Fatal("cleanup blocked the other game", s)
	}

	kernel.setError = nil

	if s := status(d); s.Error != "" {
		t.Fatal("cleanup failed to recover", s)
	}

	if _, err := os.Stat(stage); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("orphaned staging directory", err)
	}

	for _, limit := range kernel.limits {
		if limit.Hard != 0 {
			t.Fatal("retired stage retained its quota", limit)
		}
	}
}

func TestFullBtrfsRescanRetriesProvisioning(t *testing.T) {
	for _, flags := range []uint64{3, 5, 7} {
		if err := checkQgroupStatus(1, flags); !errors.Is(err, errQgroupRescan) {
			t.Fatalf("full qgroup status %#x should retry: %v", flags, err)
		}
	}

	for _, flags := range []uint64{0, 9, 11} {
		if err := checkQgroupStatus(1, flags); err == nil || errors.Is(err, errQgroupRescan) {
			t.Fatalf("disabled or simple qgroup status %#x accepted as full quotas", flags)
		}
	}

	m, d := fileTestManager(t)
	q := &fakeQuota{err: quotaError(checkQgroupStatus(1, 3))}
	m.setInitialQuota(q, q.err)

	m.Ownership = func(string, string) error { return nil }
	if m.DiskQuotaSupported() {
		t.Fatal("reported enforcement while rescan was in progress")
	}

	if s := awaitOperation(t, m, d, "install", "during-rescan"); s.OperationState != model.OperationFailed || s.DiskQuotaErrorCode != model.RuntimeQuotaUnavailable || s.DiskQuotaEnforced {
		t.Fatal("installation proceeded without enforcement", s)
	}

	q.err = checkQgroupStatus(1, 1)

	if s := awaitOperation(t, m, d, "install", "after-rescan"); s.Error != "" || !s.DiskQuotaEnforced || !m.DiskQuotaSupported() {
		t.Fatal("installation did not recover after the rescan", s)
	}
}

func TestQuotaAllocationVerificationAndResize(t *testing.T) {
	q, k, directory := quotaFixture(t)
	if err := q.Verify(t.Context(), directory, 8); err == nil {
		t.Fatal("unprepared directory reported enforced")
	}

	if err := q.Prepare(t.Context(), directory, 8); err != nil {
		t.Fatal(err)
	}

	if err := q.Verify(t.Context(), directory, 8); err != nil {
		t.Fatal(err)
	}

	if err := q.Verify(t.Context(), directory, 16); err == nil {
		t.Fatal("old allocation reported enforced")
	}

	var id uint32
	for project := range k.limits {
		id = project
	}

	if id < 0x40000000 || id > 0x7fffffff || k.limits[id].Hard != 8192 {
		t.Fatal("wrong ID range or MiB conversion")
	}

	limit := k.limits[id]
	limit.Space = 6 << 20
	k.limits[id] = limit

	if err := q.Prepare(t.Context(), directory, 4); err == nil {
		t.Fatal("shrank below charged usage")
	}

	if k.limits[id].Hard != 8192 {
		t.Fatal("failed shrink changed kernel limit")
	}

	if err := q.Prepare(t.Context(), directory, 16); err != nil {
		t.Fatal(err)
	}

	restarted := &projectQuotas{registry: q.registry, kernel: k}
	if err := restarted.Verify(t.Context(), directory, 16); err != nil {
		t.Fatal(err)
	}

	k.disabled = true

	if err := restarted.Verify(t.Context(), directory, 16); err == nil {
		t.Fatal("disabled enforcement reported enforced")
	}
}

func TestQuotaRefusesUnsafeAdoptionAndFailedReadback(t *testing.T) {
	for _, scenario := range []string{"nonempty", "foreign", "symlink", "set failure", "readback failure"} {
		t.Run(scenario, func(t *testing.T) {
			q, k, directory := quotaFixture(t)

			switch scenario {
			case "nonempty":
				if err := os.WriteFile(filepath.Join(directory, "existing"), []byte("data"), 0600); err != nil {
					t.Fatal(err)
				}
			case "foreign":
				f, err := os.Open(directory)
				if err != nil {
					t.Fatal(err)
				}

				k.attrs[inode(f)] = projectAttributes{Project: 42, Flags: 0x200}
				_ = f.Close()
			case "symlink":
				link := directory + "-link"
				if err := os.Symlink(directory, link); err != nil {
					t.Fatal(err)
				}

				directory = link
			case "set failure":
				k.setError = syscall.EPERM
			case "readback failure":
				k.ignoreSet = true
			}

			if err := q.Prepare(t.Context(), directory, 8); err == nil {
				t.Fatal("unsafe quota setup succeeded")
			}

			if err := q.Verify(t.Context(), directory, 8); err == nil {
				t.Fatal("failed quota setup reported enforced")
			}
		})
	}
}

func TestQuotaRestoreProjectsFollowRenameAndAreNotRecycled(t *testing.T) {
	q, k, directory := quotaFixture(t)
	if err := q.Prepare(t.Context(), directory, 8); err != nil {
		t.Fatal(err)
	}

	stage := filepath.Join(filepath.Dir(directory), ".restore-stage")
	if err := os.Mkdir(stage, 0700); err != nil {
		t.Fatal(err)
	}

	if err := q.Prepare(t.Context(), stage, 8); err != nil {
		t.Fatal(err)
	}

	if len(k.limits) != 2 {
		t.Fatal("restore shares original quota")
	}

	previous := directory + "-previous"
	if err := os.Rename(directory, previous); err != nil {
		t.Fatal(err)
	}

	if err := os.Rename(stage, directory); err != nil {
		t.Fatal(err)
	}

	if err := q.Verify(t.Context(), directory, 8); err != nil {
		t.Fatal(err)
	}

	if err := q.Verify(t.Context(), previous, 8); err != nil {
		t.Fatal(err)
	}

	file, err := os.Open(previous)
	if err != nil {
		t.Fatal(err)
	}

	retiredID := k.attrs[inode(file)].Project
	_ = file.Close()

	k.setError = syscall.EPERM
	if err := removeQuotaData(t.Context(), q, previous); !errors.Is(err, syscall.EPERM) {
		t.Fatalf("failed retirement should preserve the old directory for a retry: %v", err)
	}

	k.setError = nil

	if err := removeQuotaData(t.Context(), q, previous); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(previous); !errors.Is(err, os.ErrNotExist) || k.limits[retiredID].Hard != 0 {
		t.Fatal("retired directory or hard limit survived", err, k.limits[retiredID])
	}

	if err := q.Verify(t.Context(), directory, 8); err != nil {
		t.Fatal(err)
	}

	records, err := filepath.Glob(filepath.Join(q.registry, "*.json"))
	if err != nil || len(records) != 2 {
		t.Fatal("quota identities were recycled")
	}
}

func TestLifecycleUsesAllocatedBytesForSparseGames(t *testing.T) {
	m, d := fileTestManager(t)
	m.Quota = &accountingQuota{DiskQuota: m.Quota, bytes: 4096}
	m.Ownership = func(string, string) error { return nil }
	d.Configuration.DiskMB = 8

	save := filepath.Join(m.root(d.ServerID), "data", "world")
	if err := os.WriteFile(save, nil, 0600); err != nil {
		t.Fatal(err)
	}

	if err := os.Truncate(save, 16<<20); err != nil {
		t.Fatal(err)
	}

	for i, step := range []string{"install", "start", "stop", "start"} {
		status := awaitOperation(t, m, d, step, step+strconv.Itoa(i))
		if status.Error != "" {
			t.Fatal(step, status.Error)
		}
	}
}

func TestQuotaFailurePreventsInstallerAndGameStart(t *testing.T) {
	m := newTestManager(context.Background(), t.TempDir())
	t.Cleanup(m.Close)

	docker := &fakeDocker{states: make(map[string]string)}
	m.Docker = docker
	m.Quota = fakeQuota{err: errors.New("project quotas disabled")}

	d := runtimeFixture()
	if status := awaitOperation(t, m, d, "install", "quota-failure"); !strings.Contains(status.Error, "quotas disabled") || status.DiskQuotaEnforced {
		t.Fatalf("installer did not fail closed: %+v", status)
	}

	entry, err := m.load(d.ServerID)
	if err != nil {
		t.Fatal(err)
	}

	entry.mu.Lock()
	entry.Installed = true
	entry.mu.Unlock()

	if status := awaitOperation(t, m, d, "start", "start-failure"); !strings.Contains(status.Error, "quotas disabled") || status.DiskQuotaEnforced {
		t.Fatalf("start did not fail closed: %+v", status)
	}

	docker.mu.Lock()
	defer docker.mu.Unlock()

	for _, args := range docker.calls {
		if args[0] == "run" || args[0] == "create" || args[0] == "start" {
			t.Fatal("container launched without quota")
		}
	}
}

func TestQuotaProbeChecksPermissionsAndEnforcement(t *testing.T) {
	for _, scenario := range []string{"supported", "unsupported", "permission denied", "not enforced"} {
		t.Run(scenario, func(t *testing.T) {
			quota, kernel, directory := quotaFixture(t)
			kernel.disabled = scenario == "unsupported"

			kernel.ignoreSet = scenario == "not enforced"
			if scenario == "permission denied" {
				kernel.setError = syscall.EPERM
			}

			err := probeDiskQuota(t.Context(), filepath.Dir(directory), quota, 1)
			if (err == nil) != (scenario == "supported") {
				t.Fatal("incorrect quota detection", err)
			}

			probes, err := filepath.Glob(filepath.Join(filepath.Dir(directory), ".quota-probe-*"))
			if err != nil || len(probes) != 0 {
				t.Fatal("quota probe left a temporary directory", probes, err)
			}
		})
	}
}

func TestUnlimitedQuotaClearsLimitAndRetainsProjectIdentity(t *testing.T) {
	for _, initial := range []int64{0, 8} {
		quota, kernel, directory := quotaFixture(t)
		if err := quota.Prepare(t.Context(), directory, initial); err != nil {
			t.Fatal(err)
		}

		var id uint32
		for _, attributes := range kernel.attrs {
			id = attributes.Project
		}

		limit := kernel.limits[id]
		limit.Space = 16 << 20
		kernel.limits[id] = limit

		if err := os.WriteFile(filepath.Join(directory, "world"), []byte("game data"), 0600); err != nil {
			t.Fatal(err)
		}

		if err := quota.Prepare(t.Context(), directory, 0); err != nil {
			t.Fatal("could not remove quota", err)
		}

		if err := quota.Verify(t.Context(), directory, 0); err != nil || kernel.limits[id].Hard != 0 {
			t.Fatal("unlimited allocation retained a hard limit", err)
		}

		if err := quota.Prepare(t.Context(), directory, 8); err == nil {
			t.Fatal("reapplied quota smaller than current usage")
		}

		limit = kernel.limits[id]
		limit.Space = 4 << 20
		kernel.limits[id] = limit

		if err := quota.Prepare(t.Context(), directory, 8); err != nil || kernel.limits[id].Hard != 8*1024 {
			t.Fatal("could not reapply quota to an existing unlimited world", err)
		}
	}
}

func TestLifecycleWithoutQuotaSupport(t *testing.T) {
	for _, allocation := range []int64{0, 1024} {
		m := newTestManager(t.Context(), t.TempDir())
		t.Cleanup(m.Close)
		m.Quota = nil
		m.Docker = &fakeDocker{states: make(map[string]string)}
		m.Ownership = func(string, string) error { return nil }

		d := runtimeFixture()
		d.Configuration.DiskMB = allocation

		for _, action := range []string{"install", "start", "stop"} {
			status := awaitOperation(t, m, d, action, action)
			if status.Error != "" || status.DiskQuotaEnforced {
				t.Fatal("quota-less lifecycle failed or claimed enforcement", action, status)
			}
		}
	}
}

func TestQuotaFailureBlocksWritesButAllowsDeletion(t *testing.T) {
	m, d := fileTestManager(t)
	m.Quota = fakeQuota{err: syscall.EOPNOTSUPP}

	name := filepath.Join(m.root(d.ServerID), "data", "remove-me")
	if err := os.WriteFile(name, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := m.Files(context.Background(), d, model.FileRequest{ServerID: d.ServerID, Operation: "write", Path: "new", Data: []byte("data"), Revision: "absent"})
	if err == nil {
		t.Fatal("wrote without quota")
	}

	if _, err := m.Files(context.Background(), d, model.FileRequest{ServerID: d.ServerID, Operation: "delete", Path: "remove-me"}); err != nil {
		t.Fatal(err)
	}

	usage, err := m.Measure(context.Background(), d)
	if err != nil || usage.DiskQuotaEnforced || usage.DiskQuotaError == "" {
		t.Fatal("monitoring did not report unenforced quota")
	}
}

func TestQuotaKernelABISizesAndSeccompProtection(t *testing.T) {
	if unsafe.Sizeof(projectAttributes{}) != 28 || unsafe.Sizeof(projectLimit{}) != 72 {
		t.Fatal("Linux quota ABI mismatch")
	}

	if unsafe.Offsetof(projectLimit{}.Space) != 16 || unsafe.Offsetof(projectLimit{}.Valid) != 64 || unsafe.Offsetof(projectAttributes{}.Project) != 12 {
		t.Fatal("Linux quota ABI offsets mismatch")
	}

	data, err := quotaSeccomp()
	if err != nil {
		t.Fatal(err)
	}

	var profile struct {
		DefaultAction string
		Syscalls      []struct {
			Names  []string
			Action string
			Args   []struct {
				Index           int
				Value, ValueTwo uint64
				Op              string
			}
		}
	}
	if err := json.Unmarshal(data, &profile); err != nil {
		t.Fatal(err)
	}

	if profile.DefaultAction != "SCMP_ACT_ERRNO" {
		t.Fatal("Docker default allowlist was lost")
	}

	allowed := func(command uint64) bool {
		for _, rule := range profile.Syscalls {
			for _, name := range rule.Names {
				if name != "ioctl" || rule.Action != "SCMP_ACT_ALLOW" {
					continue
				}

				if len(rule.Args) != 1 {
					t.Fatal("unrestricted ioctl rule")
				}

				arg := rule.Args[0]
				if arg.Index != 1 || arg.Op != "SCMP_CMP_MASKED_EQ" {
					t.Fatal("incorrect ioctl argument filter")
				}

				if command&arg.Value == arg.ValueTwo {
					return true
				}
			}
		}

		return false
	}
	for _, command := range []uint64{0x401c5820, 0x40086602, 0x40046602, 0xffffffff401c5820, 0xffffffff40086602, 0xffffffff40046602} {
		if allowed(command) {
			t.Fatalf("quota attribute mutation %#x is allowed", command)
		}

		if !allowed(command-1) || !allowed(command+1) {
			t.Fatalf("unrelated ioctl near %#x is denied", command)
		}
	}

	for _, command := range []uint64{0, 0x801c581f, 0x5401, 0x541b, 0xffffffff} {
		if !allowed(command) {
			t.Fatalf("unrelated ioctl %#x is denied", command)
		}
	}

	for _, command := range []uint64{0x50009401, 0x5000940e, 0x50009417, 0x50009418} {
		if allowed(command) || allowed(command|0xffffffff00000000) {
			t.Fatalf("nested Btrfs subvolume creation %#x allowed", command)
		}
	}
}
