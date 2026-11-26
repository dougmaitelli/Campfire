package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// OpenZFS shares the Linux project attribute ioctls with ext4 and XFS.
// Its project limits and counters are dataset properties, expressed in bytes.
type zfsProjectKernel struct {
	linuxProjectKernel
	directory string
	device    uint64
}

// OpenZFS 2.2 uses the inode flag FS_PROJINHERIT_FL in fsx_xflags.
// Newer versions use the Linux FS_XFLAG_PROJINHERIT encoding.
const zfsLegacyProjectInherit = 0x20000000

func (k *zfsProjectKernel) Attributes(f *os.File) (projectAttributes, error) {
	attrs, err := k.linuxProjectKernel.Attributes(f)
	if attrs.Flags&zfsLegacyProjectInherit != 0 {
		attrs.Flags = attrs.Flags&^zfsLegacyProjectInherit | projectInherit
	}

	return attrs, err
}

func (k *zfsProjectKernel) Assign(f *os.File, attrs projectAttributes) error {
	err := k.linuxProjectKernel.Assign(f, attrs)
	if errors.Is(err, syscall.EOPNOTSUPP) && attrs.Flags&projectInherit != 0 {
		attrs.Flags = attrs.Flags&^projectInherit | zfsLegacyProjectInherit
		return k.linuxProjectKernel.Assign(f, attrs)
	}

	return err
}

func newZFSProjectKernel(directory string) (*zfsProjectKernel, error) {
	path, err := filepath.Abs(directory)
	if err != nil {
		return nil, err
	}

	var stat syscall.Stat_t
	if err := syscall.Stat(directory, &stat); err != nil {
		return nil, err
	}

	return &zfsProjectKernel{directory: path, device: stat.Dev}, nil
}

func (k *zfsProjectKernel) Enabled(f *os.File) error {
	var stat syscall.Stat_t
	if err := syscall.Fstat(int(f.Fd()), &stat); err != nil {
		return err
	}

	if stat.Dev != k.device {
		return errors.New("game data must reside on the Spark runtime ZFS dataset")
	}

	return nil
}

func (k *zfsProjectKernel) Get(ctx context.Context, _ *os.File, id uint32) (projectLimit, error) {
	names := []string{"projectquota", "projectused", "projectobjquota", "projectobjused"}
	for i := range names {
		names[i] += "@" + strconv.FormatUint(uint64(id), 10)
	}

	output, err := quotaCommand(ctx, "zfs", "get", "-Hp", "-o", "property,value", strings.Join(names, ","), k.directory)
	if err != nil {
		return projectLimit{}, err
	}

	values := make(map[string]uint64, len(names))

	for line := range strings.Lines(output) {
		name, raw, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if !ok {
			return projectLimit{}, errors.New("invalid ZFS project property")
		}

		if raw == "none" && (name == names[0] || name == names[2]) {
			raw = "0"
		}

		value, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return projectLimit{}, fmt.Errorf("ZFS property %s unavailable: %w", name, err)
		}

		values[name] = value
	}

	for _, name := range names {
		if _, ok := values[name]; !ok {
			return projectLimit{}, fmt.Errorf("ZFS property %s unavailable", name)
		}
	}

	hard := values[names[0]]
	// Keep non-KiB external limits distinguishable from a free project or an exact allocation.
	if hard%1024 != 0 {
		return projectLimit{}, errors.New("ZFS project has a non-KiB quota")
	}

	return projectLimit{Hard: hard / 1024, Space: values[names[1]], InodeHard: values[names[2]], Inodes: values[names[3]]}, nil
}

func (k *zfsProjectKernel) Set(ctx context.Context, _ *os.File, id uint32, limit projectLimit) error {
	value := "none"
	if limit.Hard != 0 {
		value = strconv.FormatUint(limit.Hard*1024, 10)
	}
	// zfs set takes a dataset name. Resolve it at mutation time so renaming
	// a mounted dataset does not redirect changes to a cached name.
	dataset, err := quotaCommand(ctx, "zfs", "list", "-H", "-o", "name", k.directory)
	if err != nil {
		return err
	}

	if dataset == "" || strings.ContainsAny(dataset, "\r\n") || strings.HasPrefix(dataset, "-") {
		return errors.New("could not identify ZFS dataset")
	}

	_, err = quotaCommand(ctx, "zfs", "set", fmt.Sprintf("projectquota@%d=%s", id, value), dataset)

	return err
}
