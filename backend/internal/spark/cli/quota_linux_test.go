package cli

import (
	"os"
	"os/exec"
	"testing"

	"golang.org/x/sys/unix"
)

func TestSparkDropsQuotaOverride(t *testing.T) {
	header := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}

	var caps [2]unix.CapUserData
	if err := unix.Capget(&header, &caps[0]); err != nil {
		t.Fatal(err)
	}

	const child = "CAMPFIRE_TEST_QUOTA_OVERRIDE_CHILD"
	if os.Getenv(child) == "1" {
		if err := dropQuotaOverride(); err != nil {
			t.Fatal(err)
		}

		if err := unix.Capget(&header, &caps[0]); err != nil || caps[0].Effective&(1<<unix.CAP_SYS_RESOURCE) != 0 {
			t.Fatal("Spark still has the quota override capability", err)
		}

		return
	}

	if caps[0].Effective&(1<<unix.CAP_SYS_RESOURCE) == 0 {
		t.Skip("run with CAP_SYS_RESOURCE to exercise a privileged Spark launch")
	}

	cmd := exec.CommandContext(t.Context(), "/proc/self/exe", "-test.run=^TestSparkDropsQuotaOverride$")

	cmd.Env = append(os.Environ(), child+"=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s: %v", output, err)
	}
}
