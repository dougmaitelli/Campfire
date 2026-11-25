package runtime

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"campfire/internal/shared/model"
)

// DiskQuota prepares and verifies persistent game-data limits. Zero removes the
// limit; positive allocations are enforced independently of the Spark process.
type DiskQuota interface {
	Prepare(context.Context, string, int64) error
	Verify(context.Context, string, int64) error
}

// DiskQuotaSupported reports actual filesystem enforcement, including host permissions.
func (m *Manager) DiskQuotaSupported() bool { return m.Quota != nil && !m.pendingQuotaProbe.Load() }

func (m *Manager) setInitialQuota(quota DiskQuota, probeErr error) {
	if probeErr == nil || errors.Is(probeErr, errQgroupRescan) {
		m.Quota = quota
		m.pendingQuotaProbe.Store(probeErr != nil)

		return
	}

	// Existing ZFS projects must still be retired if the CLI was unavailable at startup.
	if q, ok := quota.(*projectQuotas); ok {
		if _, zfs := q.kernel.(*zfsProjectKernel); zfs {
			m.retireProjects = q
		}
	}

	m.quotaUnavailable = probeErr.Error()
}

func probeDiskQuota(ctx context.Context, directory string, quota DiskQuota, megabytes int64) (result error) {
	if quota == nil {
		return nil
	}

	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}

	probe, err := os.MkdirTemp(directory, ".quota-probe-")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, removeQuotaData(context.WithoutCancel(ctx), quota, probe)) }()

	if err := quota.Prepare(ctx, probe, megabytes); err != nil {
		return err
	}

	if err := quota.Verify(ctx, probe, megabytes); err != nil {
		return err
	}

	if megabytes == 0 {
		return nil
	}

	return quota.Prepare(ctx, probe, 0)
}

func (m *Manager) prepareQuota(ctx context.Context, directory string, megabytes int64) error {
	if m.Quota == nil {
		return nil
	}

	if err := m.retryQuotaProbe(ctx); err != nil {
		return err
	}

	return m.Quota.Prepare(ctx, directory, megabytes)
}

func (m *Manager) retryQuotaProbe(ctx context.Context) error {
	if !m.pendingQuotaProbe.Load() {
		return nil
	}

	if err := probeDiskQuota(ctx, m.Directory, m.Quota, 1); err != nil {
		return err
	}

	m.pendingQuotaProbe.Store(false)

	return nil
}

// A running container can retain a bind mount to the original inode even if
// the host replaces the directory. Only resize in place on these paths.
func (m *Manager) resizeMountedQuota(ctx context.Context, directory string, megabytes int64) error {
	if q, ok := m.Quota.(*btrfsQuotas); ok {
		if err := m.retryQuotaProbe(ctx); err != nil {
			return err
		}

		return q.limit(ctx, directory, megabytes, resizeQgroup)
	}

	return m.prepareQuota(ctx, directory, megabytes)
}

type projectAttributes struct {
	Flags, ExtentSize, Extents, Project, CowExtentSize uint32
	Padding                                            [8]byte
}

const projectInherit = 0x200 // FS_XFLAG_PROJINHERIT

type projectLimit struct {
	Hard, Soft, Space, InodeHard, InodeSoft, Inodes, BlockTime, InodeTime uint64
	Valid                                                                 uint32
}

type projectKernel interface {
	Enabled(*os.File) error
	Attributes(*os.File) (projectAttributes, error)
	Assign(*os.File, projectAttributes) error
	Get(context.Context, *os.File, uint32) (projectLimit, error)
	Set(context.Context, *os.File, uint32, projectLimit) error
}

type projectQuotas struct {
	registry string
	kernel   projectKernel
}

type projectOwner struct {
	Device uint64 `json:"-"`
	Inode  uint64 `json:"inode"`
}

func quotaError(err error) error {
	return &runtimeFailure{code: model.RuntimeQuotaUnavailable, cause: err, message: fmt.Sprintf("Disk quota enforcement is unavailable: %v", err)}
}

func openProjectDir(directory string) (*os.File, projectOwner, error) {
	file, err := openQuotaDir(directory)
	if err != nil {
		return nil, projectOwner{}, err
	}

	var stat syscall.Stat_t
	if err := syscall.Fstat(int(file.Fd()), &stat); err != nil {
		_ = file.Close()
		return nil, projectOwner{}, err
	}

	return file, projectOwner{Device: stat.Dev, Inode: stat.Ino}, nil
}

func (q *projectQuotas) open(directory string) (*os.File, projectOwner, error) {
	file, owner, err := openProjectDir(directory)
	if err != nil {
		return nil, projectOwner{}, err
	}

	if err := q.kernel.Enabled(file); err != nil {
		_ = file.Close()
		return nil, projectOwner{}, err
	}

	return file, owner, nil
}

func (q *projectQuotas) owner(id uint32) string {
	return filepath.Join(q.registry, strconv.FormatUint(uint64(id), 10)+".json")
}

func (q *projectQuotas) owned(id uint32, owner projectOwner) bool {
	owned, err := q.readOwner(id, owner)
	return err == nil && owned
}

func (q *projectQuotas) readOwner(id uint32, owner projectOwner) (bool, error) {
	data, err := os.ReadFile(q.owner(id))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}

	if err != nil {
		return false, err
	}
	var recorded projectOwner
	if err := json.Unmarshal(data, &recorded); err != nil {
		return false, err
	}

	return recorded.Inode == owner.Inode && q.sameFilesystem(owner), nil
}

// The registry is private to Spark. Lock the directory's inode there so a
// rename preserves the lock and unrelated games can reconcile independently.
// The permanent ID records still reserve new IDs atomically with O_EXCL.
func (q *projectQuotas) lockDirectory(ctx context.Context, owner projectOwner) (*os.File, error) {
	name := filepath.Join(q.registry, fmt.Sprintf(".inode-%d.lock", owner.Inode))

	lock, err := os.OpenFile(name, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}

	for {
		if err = ctx.Err(); err != nil {
			break
		}

		err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return lock, nil
		}

		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EINTR) {
			break
		}

		select {
		case <-ctx.Done():
		case <-time.After(20 * time.Millisecond):
		}
	}

	_ = lock.Close()

	return nil, err
}

// The limit belongs to the filesystem. Keep the ID reservation after unlink
// so a deleted-open file can never be charged to a different game.
func removeQuotaData(ctx context.Context, quota DiskQuota, directory string) error {
	if q, ok := quota.(*projectQuotas); ok {
		if err := q.retire(ctx, directory); err != nil {
			return err
		}
	}

	return os.RemoveAll(directory)
}

func (q *projectQuotas) retire(ctx context.Context, directory string) error {
	if _, err := os.Stat(q.registry); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return err
	}

	file, owner, err := openProjectDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err != nil {
		return err
	}

	defer func() { _ = file.Close() }()

	if !q.sameFilesystem(owner) {
		return nil
	}

	lock, err := q.lockDirectory(ctx, owner)
	if err != nil {
		return err
	}

	defer func() { _ = lock.Close() }()

	attrs, err := q.kernel.Attributes(file)
	if err != nil {
		return err
	}

	if attrs.Project == 0 {
		return nil
	}

	owned, err := q.readOwner(attrs.Project, owner)
	if err != nil || !owned {
		return err
	}

	if err := q.kernel.Enabled(file); err != nil {
		return quotaError(err)
	}

	if err := q.kernel.Set(ctx, file, attrs.Project, projectLimit{Valid: 1}); err != nil && !errors.Is(err, os.ErrNotExist) {
		return quotaError(err)
	}

	return nil
}

func (m *Manager) removeGameData(ctx context.Context, directory string) error {
	if m.Quota == nil && m.retireProjects != nil {
		return removeQuotaData(ctx, m.retireProjects, directory)
	}

	return removeQuotaData(ctx, m.Quota, directory)
}

func (q *projectQuotas) sameFilesystem(owner projectOwner) bool {
	// Device numbers can change across host reboots. Compare the live devices
	// of registry and game data instead of persisting a transient device number.
	var stat syscall.Stat_t
	return syscall.Stat(q.registry, &stat) == nil && stat.Dev == owner.Device
}

func (q *projectQuotas) Verify(ctx context.Context, directory string, megabytes int64) error {
	file, owner, err := q.open(directory)
	if err != nil {
		return quotaError(err)
	}
	defer func() { _ = file.Close() }()

	attrs, err := q.kernel.Attributes(file)
	if err != nil {
		return quotaError(err)
	}

	if megabytes == 0 && attrs.Project == 0 {
		return nil
	}

	if attrs.Project == 0 || attrs.Flags&projectInherit == 0 || !q.owned(attrs.Project, owner) {
		return quotaError(errors.New("directory is not a managed quota project"))
	}

	limit, err := q.kernel.Get(ctx, file, attrs.Project)
	if megabytes == 0 && errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err != nil {
		return quotaError(err)
	}

	if megabytes < 0 || megabytes > 1<<30 || limit.Hard != uint64(megabytes)*1024 || limit.Soft != 0 {
		return quotaError(errors.New("filesystem hard limit does not match the server allocation"))
	}

	return nil
}

func (q *projectQuotas) Prepare(ctx context.Context, directory string, megabytes int64) error {
	if megabytes < 0 || megabytes > 1<<30 {
		return errors.New("invalid disk allocation")
	}

	file, owner, err := q.open(directory)
	if err != nil {
		return quotaError(err)
	}

	defer func() { _ = file.Close() }()

	if err := os.MkdirAll(q.registry, 0700); err != nil {
		return quotaError(err)
	}

	if !q.sameFilesystem(owner) {
		return quotaError(errors.New("quota registry and game data must share a filesystem"))
	}

	lock, err := q.lockDirectory(ctx, owner)
	if err != nil {
		return quotaError(err)
	}

	defer func() { _ = lock.Close() }()

	attrs, err := q.kernel.Attributes(file)
	if err != nil {
		return quotaError(err)
	}

	if attrs.Project != 0 {
		if !q.owned(attrs.Project, owner) || attrs.Flags&projectInherit == 0 {
			return quotaError(errors.New("refusing to modify an unmanaged quota project"))
		}
	} else {
		// Refuse adoption: existing files might have different project IDs or
		// external hard links. New directories inherit the project from birth.
		items, err := file.Readdirnames(1)
		if err != nil && !errors.Is(err, io.EOF) {
			return quotaError(err)
		}

		if len(items) != 0 {
			if megabytes == 0 {
				return nil
			}

			return quotaError(errors.New("unmanaged game-data directory must be empty"))
		}

		id, err := q.reserve(ctx, file, owner)
		if err != nil {
			return quotaError(err)
		}

		attrs.Project = id
		attrs.Flags |= projectInherit
		// Install the hard limit before assigning the first inode.
		if err := q.kernel.Set(ctx, file, id, projectLimit{Hard: uint64(megabytes) * 1024, Valid: 1}); err != nil {
			return quotaError(err)
		}

		if err := q.kernel.Assign(file, attrs); err != nil {
			clearErr := q.kernel.Set(context.WithoutCancel(ctx), file, id, projectLimit{Valid: 1})
			return quotaError(errors.Join(err, clearErr))
		}

		if err := file.Sync(); err != nil {
			return quotaError(err)
		}

		return q.Verify(ctx, directory, megabytes)
	}

	limit, err := q.kernel.Get(ctx, file, attrs.Project)
	if megabytes == 0 && errors.Is(err, os.ErrNotExist) {
		err = nil
	}

	if err != nil {
		return quotaError(err)
	}

	if megabytes > 0 && limit.Space > uint64(megabytes)<<20 {
		return errors.New("allocated filesystem blocks exceed the requested disk allocation")
	}

	if limit.Hard == uint64(megabytes)*1024 && limit.Soft == 0 {
		return nil
	}

	if err := q.kernel.Set(ctx, file, attrs.Project, projectLimit{Hard: uint64(megabytes) * 1024, Valid: 1}); err != nil {
		return quotaError(err)
	}

	return q.Verify(ctx, directory, megabytes)
}

func (q *projectQuotas) reserve(ctx context.Context, file *os.File, owner projectOwner) (uint32, error) {
	// Reserve project IDs in the Campfire range. Records are permanent: deleting
	// a directory must not recycle a quota while an unlinked file is still open.
	for range 32 {
		var random [4]byte
		if _, err := rand.Read(random[:]); err != nil {
			return 0, err
		}

		id := binary.LittleEndian.Uint32(random[:])&0x3fffffff | 0x40000000

		limit, err := q.kernel.Get(ctx, file, id)
		if errors.Is(err, os.ErrNotExist) {
			err = nil
		}

		if err != nil {
			return 0, err
		}

		if limit.Hard != 0 || limit.Soft != 0 || limit.InodeHard != 0 || limit.InodeSoft != 0 || limit.Space != 0 || limit.Inodes != 0 {
			continue
		}

		record, err := os.OpenFile(q.owner(id), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if errors.Is(err, os.ErrExist) {
			continue
		}

		if err != nil {
			return 0, err
		}

		encodeErr := json.NewEncoder(record).Encode(owner)
		syncErr := record.Sync()

		closeErr := record.Close()
		if err := errors.Join(encodeErr, syncErr, closeErr, syncBackupDirectory(q.registry)); err != nil {
			return 0, err
		}

		return id, nil
	}

	return 0, errors.New("could not reserve an unused project quota ID")
}
