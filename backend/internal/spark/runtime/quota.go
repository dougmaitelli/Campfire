package runtime

import (
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
)

// DiskQuota prepares and verifies persistent game-data limits. Implementations
// must enforce writes in the filesystem, independently of the Spark process.
type DiskQuota interface {
	Prepare(string, int64) error
	Verify(string, int64) error
}

type projectAttributes struct {
	Flags, ExtentSize, Extents, Project, CowExtentSize uint32
	Padding                                            [8]byte
}

type projectLimit struct {
	Hard, Soft, Space, InodeHard, InodeSoft, Inodes, BlockTime, InodeTime uint64
	Valid                                                                 uint32
}

type projectKernel interface {
	Enabled(*os.File) error
	Attributes(*os.File) (projectAttributes, error)
	Assign(*os.File, projectAttributes) error
	Get(*os.File, uint32) (projectLimit, error)
	Set(*os.File, uint32, projectLimit) error
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
	return fmt.Errorf("game-data quota unavailable: %w; Spark requires Linux 5.14+ and an XFS or ext4 filesystem with project quota accounting and enforcement enabled", err)
}

func (q *projectQuotas) open(directory string) (*os.File, projectAttributes, projectOwner, error) {
	fd, err := syscall.Open(directory, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, projectAttributes{}, projectOwner{}, err
	}

	file := os.NewFile(uintptr(fd), directory)

	var stat syscall.Stat_t
	if err := syscall.Fstat(fd, &stat); err != nil {
		_ = file.Close()
		return nil, projectAttributes{}, projectOwner{}, err
	}

	if err := q.kernel.Enabled(file); err != nil {
		_ = file.Close()
		return nil, projectAttributes{}, projectOwner{}, err
	}

	attrs, err := q.kernel.Attributes(file)
	if err != nil {
		_ = file.Close()
		return nil, projectAttributes{}, projectOwner{}, err
	}

	return file, attrs, projectOwner{Device: stat.Dev, Inode: stat.Ino}, nil
}

func (q *projectQuotas) owner(id uint32) string {
	return filepath.Join(q.registry, strconv.FormatUint(uint64(id), 10)+".json")
}

func (q *projectQuotas) owned(id uint32, owner projectOwner) bool {
	data, err := os.ReadFile(q.owner(id))

	var recorded projectOwner

	return err == nil && json.Unmarshal(data, &recorded) == nil && recorded.Inode == owner.Inode && q.sameFilesystem(owner)
}

func (q *projectQuotas) sameFilesystem(owner projectOwner) bool {
	// Device numbers can change across host reboots. Compare the live devices
	// of registry and game data instead of persisting a transient device number.
	var stat syscall.Stat_t
	return syscall.Stat(q.registry, &stat) == nil && stat.Dev == owner.Device
}

func (q *projectQuotas) Verify(directory string, megabytes int64) error {
	file, attrs, owner, err := q.open(directory)
	if err != nil {
		return quotaError(err)
	}
	defer func() { _ = file.Close() }()

	if attrs.Project == 0 || attrs.Flags&0x200 == 0 || !q.owned(attrs.Project, owner) {
		return quotaError(errors.New("directory is not a managed quota project"))
	}

	limit, err := q.kernel.Get(file, attrs.Project)
	if err != nil {
		return quotaError(err)
	}

	if megabytes <= 0 || megabytes > 1<<30 || limit.Hard != uint64(megabytes)*1024 || limit.Soft != 0 {
		return quotaError(errors.New("filesystem hard limit does not match the server allocation"))
	}

	return nil
}

func (q *projectQuotas) Prepare(directory string, megabytes int64) error {
	if megabytes <= 0 || megabytes > 1<<30 {
		return errors.New("invalid disk allocation")
	}

	file, attrs, owner, err := q.open(directory)
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

	lock, err := os.OpenFile(filepath.Join(q.registry, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return quotaError(err)
	}

	defer func() { _ = lock.Close() }()

	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return quotaError(err)
	}

	defer func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) }()

	if attrs.Project != 0 {
		if !q.owned(attrs.Project, owner) || attrs.Flags&0x200 == 0 {
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
			return quotaError(errors.New("unmanaged game-data directory must be empty"))
		}

		id, err := q.reserve(file, owner)
		if err != nil {
			return quotaError(err)
		}

		attrs.Project = id
		attrs.Flags |= 0x200
		// Install the hard limit before assigning the first inode.
		if err := q.kernel.Set(file, id, projectLimit{Hard: uint64(megabytes) * 1024, Valid: 1}); err != nil {
			return quotaError(err)
		}

		if err := q.kernel.Assign(file, attrs); err != nil {
			return quotaError(err)
		}

		if err := file.Sync(); err != nil {
			return quotaError(err)
		}
	}

	limit, err := q.kernel.Get(file, attrs.Project)
	if err != nil {
		return quotaError(err)
	}

	if limit.Space > uint64(megabytes)<<20 {
		return errors.New("allocated filesystem blocks exceed the requested disk allocation")
	}

	if err := q.kernel.Set(file, attrs.Project, projectLimit{Hard: uint64(megabytes) * 1024, Valid: 1}); err != nil {
		return quotaError(err)
	}

	return q.Verify(directory, megabytes)
}

func (q *projectQuotas) reserve(file *os.File, owner projectOwner) (uint32, error) {
	// Reserve project IDs in the Campfire range. Records are permanent: deleting
	// a directory must not recycle a quota while an unlinked file is still open.
	for range 32 {
		var random [4]byte
		if _, err := rand.Read(random[:]); err != nil {
			return 0, err
		}

		id := binary.LittleEndian.Uint32(random[:])&0x3fffffff | 0x40000000

		limit, err := q.kernel.Get(file, id)
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
