package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestZFSQueryHonorsSamplingDeadline(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "zfs"), []byte("#!/bin/sh\nexec /bin/sleep 10\n"), 0700); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", dir)

	k, err := newZFSProjectKernel(dir)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	if _, err := k.Get(ctx, nil, 42); !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 2*time.Second {
		t.Fatal("quota command outlived sample deadline", err)
	}
}

func TestZFSProjectProperties(t *testing.T) {
	base := t.TempDir()
	cli := filepath.Join(base, "zfs")

	script := `#!/bin/sh
case "$1" in
get)
  test "$2" = -Hp && test "$4" = property,value && test "$5" = projectquota@42,projectused@42,projectobjquota@42,projectobjused@42 && test "$6" = "$DATASET_PATH" || exit 1
  printf 'projectobjused@42\t3\nprojectquota@42\t%s\nprojectused@42\t8193\nprojectobjquota@42\t0\n' "$HARD"
  ;;
list) test "$5" = "$DATASET_PATH" && printf 'tank/spark\n' ;;
set) test "$2" = "projectquota@42=$EXPECTED" && test "$3" = tank/spark ;;
*) exit 1 ;;
esac
`
	if err := os.WriteFile(cli, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", base)
	t.Setenv("DATASET_PATH", base)
	t.Setenv("HARD", "8388608")

	k, err := newZFSProjectKernel(base)
	if err != nil {
		t.Fatal(err)
	}

	lim, err := k.Get(t.Context(), nil, 42)
	if err != nil || lim.Hard != 8192 || lim.Space != 8193 || lim.Inodes != 3 {
		t.Fatal(lim, err)
	}

	t.Setenv("EXPECTED", "8388608")

	if err := k.Set(t.Context(), nil, 42, lim); err != nil {
		t.Fatal(err)
	}

	t.Setenv("EXPECTED", "none")

	if err := k.Set(t.Context(), nil, 42, projectLimit{}); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HARD", "-")

	if _, err := k.Get(t.Context(), nil, 42); err == nil {
		t.Fatal("accepted unavailable project quota as unlimited")
	}
}
