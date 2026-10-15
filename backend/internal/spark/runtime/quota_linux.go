//go:build linux

package runtime

import (
	"errors"
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

// Linux UAPI: fs.h, quota.h and dqblk_xfs.h. Generic quota commands and
// Q_XGETQSTATV work through the filesystem's quotactl operations on ext4 and XFS.
type linuxProjectKernel struct{}

func quotaCall(file *os.File, command, id uint32, data unsafe.Pointer) error {
	// quotactl_fd is syscall 443 on the two supported release architectures.
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		return errors.New("unsupported project quota architecture")
	}

	_, _, errno := syscall.Syscall6(443, file.Fd(), uintptr(command<<8|2), uintptr(id), uintptr(data), 0, 0)
	runtime.KeepAlive(file)

	if errno != 0 {
		return errno
	}

	return nil
}

func (linuxProjectKernel) Enabled(file *os.File) error {
	var stat syscall.Statfs_t
	if err := syscall.Fstatfs(int(file.Fd()), &stat); err != nil {
		return err
	}

	if stat.Type != 0x58465342 && stat.Type != 0xef53 {
		return errors.New("game-data filesystem must be XFS or ext4")
	}
	// fs_quota_statv is 160 bytes. Flags at offset 2 report project quota
	// accounting (bit 4) and enforcement (bit 5), separately.
	var state struct {
		Version uint8
		Padding uint8
		Flags   uint16
		Rest    [156]byte
	}

	state.Version = 1
	if err := quotaCall(file, 'X'<<8|8, 0, unsafe.Pointer(&state)); err != nil {
		return err
	}

	if state.Flags&0x30 != 0x30 {
		return errors.New("project quota accounting and enforcement are not both enabled")
	}

	return nil
}

func (linuxProjectKernel) Attributes(file *os.File) (projectAttributes, error) {
	var attrs projectAttributes

	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), 0x801c581f, uintptr(unsafe.Pointer(&attrs)))
	if errno != 0 {
		return attrs, errno
	}

	return attrs, nil
}

func (linuxProjectKernel) Assign(file *os.File, attrs projectAttributes) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), 0x401c5820, uintptr(unsafe.Pointer(&attrs)))
	if errno != 0 {
		return errno
	}

	return nil
}

func (linuxProjectKernel) Get(file *os.File, id uint32) (projectLimit, error) {
	var limit projectLimit

	err := quotaCall(file, 0x800007, id, unsafe.Pointer(&limit))

	return limit, err
}

func (linuxProjectKernel) Set(file *os.File, id uint32, limit projectLimit) error {
	return quotaCall(file, 0x800008, id, unsafe.Pointer(&limit))
}
