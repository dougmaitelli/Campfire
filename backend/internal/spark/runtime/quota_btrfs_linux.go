package runtime

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"unsafe"

	"golang.org/x/sys/unix"
)

type btrfsQuotas struct{}

var errQgroupRescan = errors.New("wait for the full Btrfs quota rescan to finish and retry")

const btrfsOwner = "trusted.campfire.quota"

type qgroupAction int

const (
	verifyQgroup qgroupAction = iota
	resizeQgroup
	provisionQgroup
)

// linux/btrfs.h TREE_SEARCH v1. Query exactly one item in the quota tree
// rather than listing every server's qgroup for each usage reading.
type btrfsSearch struct {
	tree, minObject, maxObject, minOffset, maxOffset, minTrans, maxTrans uint64
	minType, maxType, items, padding                                     uint32
	reserved                                                             [4]uint64
	data                                                                 [3992]byte
}

func btrfsIoctl(f *os.File, request uintptr, data unsafe.Pointer) error {
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), request, uintptr(data))
	runtime.KeepAlive(f)

	if errno != 0 {
		return errno
	}

	return nil
}

func qgroupItem(f *os.File, kind uint32, id uint64, minSize int) ([]byte, error) {
	s := btrfsSearch{tree: 8, minOffset: id, maxOffset: id, minType: kind, maxType: kind, maxTrans: math.MaxUint64, items: 1}
	if err := btrfsIoctl(f, 0xd0009411, unsafe.Pointer(&s)); err != nil {
		return nil, fmt.Errorf("read Btrfs quota tree: %w", err)
	}
	// A native-endian 32-byte search header precedes the little-endian item.
	size := binary.NativeEndian.Uint32(s.data[28:])
	if s.items != 1 || size < uint32(minSize) || size > uint32(len(s.data)-32) {
		return nil, errors.New("missing or invalid Btrfs quota item")
	}

	return s.data[32 : 32+size], nil
}

func btrfsQuotaReady(f *os.File) error {
	item, err := qgroupItem(f, 240, 0, 24)
	if err != nil {
		return err
	}
	// Full qgroups enabled, without an in-progress rescan or inconsistent counters.
	return checkQgroupStatus(binary.LittleEndian.Uint64(item), binary.LittleEndian.Uint64(item[16:]))
}

func checkQgroupStatus(version, flags uint64) error {
	const (
		enabled = 1 << iota
		rescan
		inconsistent
		simple
	)
	if version != 1 || flags&enabled == 0 || flags&simple != 0 {
		return errors.New("enable full Btrfs quota groups and complete the quota rescan")
	}

	if flags&(rescan|inconsistent) != 0 {
		return errQgroupRescan
	}

	if flags != enabled {
		return errors.New("unsupported Btrfs quota group status flags")
	}

	return nil
}

func btrfsGroup(f *os.File) (uint64, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(int(f.Fd()), &stat); err != nil {
		return 0, err
	}

	if stat.Ino != 256 { // BTRFS_FIRST_FREE_OBJECTID, the root of a subvolume
		return 0, errProjectAccountingUnavailable
	}

	var marker [1]byte

	n, err := unix.Fgetxattr(int(f.Fd()), btrfsOwner, marker[:])
	if err != nil || n != 1 || marker[0] != 1 {
		return 0, errors.New("refusing to manage an unowned Btrfs subvolume")
	}
	// INO_LOOKUP with treeid=0 returns the containing subvolume ID.
	lookup := struct {
		tree, object uint64
		name         [4080]byte
	}{object: 256}
	if err := btrfsIoctl(f, 0xd0009412, unsafe.Pointer(&lookup)); err != nil {
		return 0, err
	}

	return lookup.tree, nil
}

func (q btrfsQuotas) DiskUsage(ctx context.Context, directory string) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	f, err := openQuotaDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return 0, errProjectAccountingUnavailable
	}

	if err != nil {
		return 0, err
	}

	defer func() { _ = f.Close() }()

	id, err := btrfsGroup(f)
	if err != nil {
		return 0, err
	}

	if err := btrfsQuotaReady(f); err != nil {
		return 0, err
	}

	return qgroupUsage(f, id)
}

func qgroupUsage(f *os.File, id uint64) (int64, error) {
	info, err := qgroupItem(f, 242, id, 16)
	if err != nil {
		return 0, err
	}

	used := binary.LittleEndian.Uint64(info[8:])
	if used > math.MaxInt64 {
		return 0, errors.New("invalid Btrfs referenced byte count")
	}

	return int64(used), nil
}

func (q btrfsQuotas) Verify(ctx context.Context, directory string, mb int64) error {
	return q.limit(ctx, directory, mb, verifyQgroup)
}

func (q btrfsQuotas) Prepare(ctx context.Context, directory string, mb int64) error {
	return q.limit(ctx, directory, mb, provisionQgroup)
}

func (q btrfsQuotas) limit(ctx context.Context, directory string, mb int64, action qgroupAction) error {
	if mb < 0 || mb > 1<<30 {
		return errors.New("invalid disk allocation")
	}

	f, err := openQuotaDir(directory)
	if err != nil {
		return quotaError(err)
	}

	defer func() { _ = f.Close() }()

	if err := btrfsQuotaReady(f); err != nil {
		return quotaError(err)
	}

	id, err := btrfsGroup(f)
	if errors.Is(err, errProjectAccountingUnavailable) {
		if action == provisionQgroup {
			_, err := f.Readdirnames(1)
			if err == nil && mb == 0 {
				return nil
			}

			if !errors.Is(err, io.EOF) {
				return quotaError(errors.New("game-data directory must be empty to create a Btrfs quota group"))
			}

			return q.create(ctx, directory, mb)
		}

		if mb == 0 {
			return nil
		}

		return quotaError(errors.New("game data must be prepared as a Btrfs subvolume before mounting it"))
	}

	if err != nil {
		return quotaError(err)
	}

	limit, err := qgroupItem(f, 244, id, 16)
	if err != nil {
		return quotaError(err)
	}

	flags := binary.LittleEndian.Uint64(limit)

	hard := binary.LittleEndian.Uint64(limit[8:])
	if (mb == 0 && flags&1 == 0) || (mb > 0 && flags&1 != 0 && hard == uint64(mb)<<20) {
		return nil
	}

	if action == verifyQgroup {
		return quotaError(errors.New("the Btrfs referenced limit differs from the server allocation"))
	}

	size := "none"

	if mb > 0 {
		// Flush delayed accounting before deciding whether a shrink fits.
		if err := unix.Syncfs(int(f.Fd())); err != nil {
			return quotaError(err)
		}

		used, err := qgroupUsage(f, id)
		if err != nil {
			return quotaError(err)
		}

		if used > mb<<20 {
			return errors.New("referenced Btrfs space exceeds the requested disk allocation")
		}

		size = strconv.FormatInt(mb<<20, 10)
	}

	if _, err := quotaCommand(ctx, "btrfs", "qgroup", "limit", "--", size, "0/"+strconv.FormatUint(id, 10), directory); err != nil {
		return quotaError(err)
	}

	if err := unix.Syncfs(int(f.Fd())); err != nil {
		return quotaError(err)
	}

	return q.Verify(ctx, directory, mb)
}

func (q btrfsQuotas) create(ctx context.Context, directory string, mb int64) error {
	stage, err := os.MkdirTemp(filepath.Dir(directory), ".btrfs-")
	if err != nil {
		return quotaError(err)
	}
	defer func() { _ = os.RemoveAll(stage) }()

	sub := filepath.Join(stage, "data")
	if _, err := quotaCommand(ctx, "btrfs", "subvolume", "create", "--", sub); err != nil {
		return quotaError(err)
	}

	if err := os.Chmod(sub, 0700); err != nil {
		return quotaError(err)
	}

	if err := unix.Setxattr(sub, btrfsOwner, []byte{1}, unix.XATTR_CREATE); err != nil {
		return quotaError(err)
	}

	if err := q.Prepare(ctx, sub, mb); err != nil {
		return err
	}
	// rename(2) can replace an empty ordinary directory. A racing write or an
	// existing subvolume prevents publication without disturbing its contents.
	if err := unix.Rename(sub, directory); err != nil {
		return quotaError(err)
	}

	return syncBackupDirectory(filepath.Dir(directory))
}
