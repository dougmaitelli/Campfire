package cli

import (
	"os"
	"runtime"

	"golang.org/x/sys/unix"
)

// A root service can override project hard limits on ext4. Drop that privilege
// before Spark starts any workers. Capability changes apply to one OS thread,
// so re-exec from that thread to give every worker the same credentials.
func dropQuotaOverride() error {
	runtime.LockOSThread()

	defer runtime.UnlockOSThread()

	header := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}

	var caps [2]unix.CapUserData
	if err := unix.Capget(&header, &caps[0]); err != nil {
		return err
	}

	const override = 1 << unix.CAP_SYS_RESOURCE
	if caps[0].Effective&override == 0 {
		return nil
	}

	// Prevent exec as UID 0 from reacquiring the capability or gaining any
	// capabilities beyond those granted to the service by its operator.
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return err
	}

	caps[0].Effective &^= override
	caps[0].Permitted &^= override

	caps[0].Inheritable &^= override
	if err := unix.Capset(&header, &caps[0]); err != nil {
		return err
	}

	return unix.Exec("/proc/self/exe", os.Args, os.Environ())
}
